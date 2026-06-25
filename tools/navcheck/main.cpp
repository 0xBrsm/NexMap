// navcheck — offline playability validator for NexMap.
//
// Builds a Recast/Detour navmesh from a map's collision triangles (Quake
// agent dimensions) and reports:
//   - walkable coverage and disconnected islands (dead geometry)
//   - reachability of each query point (spawns/items) from a reference spawn
//
// The navmesh build core (nav_mesh.cpp/.h) is shared, unmodified, with
// FrikBotNex's bot — same axis swap and agent params, so "reachable here"
// means the same thing the bots experience in-engine.
//
// Input (text, stdin or argv[1]):
//   verts <N>
//   <x> <y> <z>            x N
//   tris <M>
//   <a> <b> <c>            x M   (indices into verts)
//   points <P>
//   <label> <x> <y> <z>    x P   (label must be a single token)
//
// Output: JSON report on stdout.

#include "nav_mesh.h"
#include "nav_hull.h"
#include "nav_links.h"
#include "nav_ents.h"
#include "qworld.h"
#include "qents.h"

#include <cstdio>
#include <cstdlib>
#include <cstring>
#include <cmath>
#include <vector>
#include <string>
#include <unordered_map>
#include <queue>
#include <array>

// Hull-mode Recast params (from FrikBotNex nav_bot.cpp). Geometry comes from
// clip hull 1, which qbsp pre-expanded by the player box — so the agent is a
// POINT: walkable_radius 0 (no erosion) and walkable_height is the leftover
// hull gap (real gap minus 56), not the player height.
static void default_config(nav_mesh_build_config_t *c)
{
	c->cell_size = 4.0f;
	c->cell_height = 2.0f;
	c->walkable_slope_angle = 45.0f;
	c->walkable_height = 8.0f;
	c->walkable_climb = 18.0f;
	c->walkable_radius = 0.0f;
	c->max_edge_len = 192.0f;
	c->max_simplification_error = 0.1f;
	c->min_region_size = 2;
	c->merge_region_size = 20;
	c->max_verts_per_poly = 6;
	c->detail_sample_distance = 6.0f;
	c->detail_sample_max_error = 1.0f;
}

struct QueryPoint {
	std::string label;
	float pos[3];
	bool snapped = false;
	float snap_dist = 0.0f;
	int island = -1;
	bool reachable = false;
};

static bool read_input(FILE *f, std::vector<float> &verts, std::vector<int> &tris,
	std::vector<QueryPoint> &points, std::vector<nav_off_mesh_link_t> &links,
	char *err, size_t errsz)
{
	char tok[64];
	int n;
	if (fscanf(f, "%63s %d", tok, &n) != 2 || strcmp(tok, "verts") != 0) {
		snprintf(err, errsz, "expected 'verts <N>'");
		return false;
	}
	verts.resize((size_t)n * 3);
	for (int i = 0; i < n; i++) {
		if (fscanf(f, "%f %f %f", &verts[i*3], &verts[i*3+1], &verts[i*3+2]) != 3) {
			snprintf(err, errsz, "bad vertex %d", i);
			return false;
		}
	}
	if (fscanf(f, "%63s %d", tok, &n) != 2 || strcmp(tok, "tris") != 0) {
		snprintf(err, errsz, "expected 'tris <M>'");
		return false;
	}
	tris.resize((size_t)n * 3);
	for (int i = 0; i < n; i++) {
		if (fscanf(f, "%d %d %d", &tris[i*3], &tris[i*3+1], &tris[i*3+2]) != 3) {
			snprintf(err, errsz, "bad triangle %d", i);
			return false;
		}
	}
	if (fscanf(f, "%63s %d", tok, &n) != 2 || strcmp(tok, "points") != 0) {
		snprintf(err, errsz, "expected 'points <P>'");
		return false;
	}
	points.resize(n);
	for (int i = 0; i < n; i++) {
		if (fscanf(f, "%63s %f %f %f", tok, &points[i].pos[0], &points[i].pos[1], &points[i].pos[2]) != 4) {
			snprintf(err, errsz, "bad point %d", i);
			return false;
		}
		points[i].label = tok;
	}
	// Optional off-mesh links: "links <L>" then L lines of
	// "sx sy sz ex ey ez radius bidir link_type". Absent = zero links.
	if (fscanf(f, "%63s %d", tok, &n) == 2 && strcmp(tok, "links") == 0) {
		links.resize(n);
		for (int i = 0; i < n; i++) {
			nav_off_mesh_link_t &l = links[i];
			memset(&l, 0, sizeof(l));
			if (fscanf(f, "%f %f %f %f %f %f %f %d %d",
				&l.start[0], &l.start[1], &l.start[2],
				&l.end[0], &l.end[1], &l.end[2],
				&l.radius, &l.bidirectional, &l.link_type) != 9) {
				snprintf(err, errsz, "bad link %d", i);
				return false;
			}
		}
	}
	return true;
}

// Connected components over the poly adjacency graph -> island id per poly.
static int label_islands(const nav_mesh_poly_record_t *recs, int count,
	std::unordered_map<unsigned long long, int> &ref2island,
	std::vector<int> &island_polys)
{
	std::unordered_map<unsigned long long, int> ref2idx;
	for (int i = 0; i < count; i++)
		ref2idx[recs[i].poly_ref] = i;

	std::vector<int> island(count, -1);
	int next = 0;
	for (int i = 0; i < count; i++) {
		if (island[i] >= 0) continue;
		std::queue<int> q;
		q.push(i);
		island[i] = next;
		while (!q.empty()) {
			int cur = q.front(); q.pop();
			for (int k = 0; k < recs[cur].neighbor_count; k++) {
				auto it = ref2idx.find(recs[cur].neighbor_refs[k]);
				if (it == ref2idx.end()) continue;
				if (island[it->second] < 0) {
					island[it->second] = next;
					q.push(it->second);
				}
			}
		}
		next++;
	}
	island_polys.assign(next, 0);
	for (int i = 0; i < count; i++) {
		ref2island[recs[i].poly_ref] = island[i];
		island_polys[island[i]]++;
	}
	return next;
}

static float poly_area(const nav_mesh_poly_record_t &r)
{
	float dx = r.bounds_max[0] - r.bounds_min[0];
	float dy = r.bounds_max[1] - r.bounds_min[1];
	return dx * dy; // approximate (bounding footprint)
}

// --- BSP mode: load geometry from the clip hull + entities directly ---

// Minimal entity-lump parsing (key "value" pairs inside { } blocks).
static std::vector<std::unordered_map<std::string,std::string>> parse_entities(const char *s)
{
	std::vector<std::unordered_map<std::string,std::string>> out;
	const char *p = s;
	while (*p) {
		if (*p != '{') { p++; continue; }
		p++;
		std::unordered_map<std::string,std::string> kv;
		while (*p && *p != '}') {
			while (*p && *p != '"' && *p != '}') p++;
			if (*p != '"') break;
			p++; std::string key; while (*p && *p != '"') key += *p++;
			if (*p == '"') p++;
			while (*p && *p != '"' && *p != '}') p++;
			if (*p != '"') break;
			p++; std::string val; while (*p && *p != '"') val += *p++;
			if (*p == '"') p++;
			kv[key] = val;
		}
		if (*p == '}') p++;
		out.push_back(std::move(kv));
	}
	return out;
}

static bool is_nav_relevant(const std::string &c)
{
	return c.rfind("info_player",0)==0 || c.rfind("weapon_",0)==0 ||
	       c.rfind("item_",0)==0 || c.rfind("ammo_",0)==0;
}

// Load hull geometry + query points + entity off-mesh links from a BSP.
// Returns the loaded world (kept active for the link callback's tracer);
// caller frees it (qworld_free + qents_free) after nav_mesh_build. NULL on fail.
static model_t *load_bsp_scene(const char *path, std::vector<float> &verts,
	std::vector<int> &tris, std::vector<QueryPoint> &points,
	std::vector<nav_off_mesh_link_t> &links, char *err, size_t errsz)
{
	model_t *world = qworld_load(path, err, (int)errsz);
	if (!world) return nullptr;
	qworld_set_active(world);
	qents_load(); // edict/sv shim over the entity lump

	// Geometry: world clip hull 1 + static brush-entity hulls (thin door
	// floors, walls, gates, trains) — walkableRadius 0, no erosion.
	float *hv = nullptr; int hvc = 0; int *ht = nullptr; int htc = 0;
	navcheck_extract_bsp(world, &hv, &hvc, &ht, &htc);
	verts.assign(hv, hv + (size_t)hvc * 3);
	tris.assign(ht, ht + (size_t)htc * 3);
	free(hv); free(ht);

	// Entity off-mesh links: teleporters, platforms, trains.
	nav_off_mesh_link_t *el = nullptr;
	int n;
	n = navcheck_collect_teleporters(&el);
	for (int i = 0; i < n; i++) links.push_back(el[i]);
	free(el); el = nullptr;
	n = navcheck_collect_platform_links(&el);
	for (int i = 0; i < n; i++) links.push_back(el[i]);
	free(el); el = nullptr;
	n = navcheck_collect_train_links(&el);
	for (int i = 0; i < n; i++) links.push_back(el[i]);
	free(el); el = nullptr;

	// Query points: spawns + pickups from the entity lump.
	auto ents = parse_entities(qworld_entities());
	std::unordered_map<std::string,int> counts;
	for (auto &kv : ents) {
		auto ci = kv.find("classname");
		if (ci == kv.end() || !is_nav_relevant(ci->second)) continue;
		auto oi = kv.find("origin");
		if (oi == kv.end()) continue;
		QueryPoint qp;
		if (sscanf(oi->second.c_str(), "%f %f %f", &qp.pos[0],&qp.pos[1],&qp.pos[2])!=3) continue;
		std::string label = ci->second.rfind("info_player",0)==0 ? "spawn" : ci->second;
		qp.label = label + "#" + std::to_string(++counts[label]);
		points.push_back(qp);
	}
	return world; // kept alive for the link callback; caller frees
}

static bool ends_with(const char *s, const char *suf)
{
	size_t ls = strlen(s), lf = strlen(suf);
	return ls >= lf && strcmp(s + ls - lf, suf) == 0;
}

// Dump the true walk graph as JSON: poly centroids + areas, base mesh
// adjacency, and off-mesh connection edges (jumps/drops/teleporters/etc).
// Consumed by tools/flowstruct.py to read macro flow structure.
static void emit_flow_graph(nav_mesh_runtime_t *nav, nav_mesh_poly_record_t *recs, int rc)
{
	auto nearest = [&](float x, float y, float z) -> int {
		int best = -1; float bd = 1e30f;
		for (int i = 0; i < rc; i++) {
			float dx = recs[i].center[0]-x, dy = recs[i].center[1]-y, dz = recs[i].center[2]-z;
			float d = dx*dx + dy*dy + dz*dz;
			if (d < bd) { bd = d; best = i; }
		}
		return best;
	};
	printf("{\n  \"polys\": [");
	for (int i = 0; i < rc; i++)
		printf("%s[%.1f,%.1f,%.1f,%.0f]", i?",":"",
			recs[i].center[0], recs[i].center[1], recs[i].center[2], poly_area(recs[i]));
	printf("],\n  \"base_edges\": [");
	std::unordered_map<unsigned long long,int> r2i;
	for (int i = 0; i < rc; i++) r2i[recs[i].poly_ref] = i;
	bool first = true;
	for (int i = 0; i < rc; i++)
		for (int k = 0; k < recs[i].neighbor_count; k++) {
			auto it = r2i.find(recs[i].neighbor_refs[k]);
			if (it == r2i.end() || it->second <= i) continue;
			printf("%s[%d,%d]", first?"":",", i, it->second); first = false;
		}
	printf("],\n  \"offmesh_edges\": [");
	const dtNavMesh *nm = nav->navmesh;
	first = true;
	for (int t = 0; t < nm->getMaxTiles(); t++) {
		const dtMeshTile *tile = nm->getTile(t);
		if (!tile || !tile->header) continue;
		for (int j = 0; j < tile->header->offMeshConCount; j++) {
			const dtOffMeshConnection *con = &tile->offMeshCons[j];
			// con->pos is Recast (x,z,y); convert to Quake (x,y,z).
			int a = nearest(con->pos[0], con->pos[2], con->pos[1]);
			int b = nearest(con->pos[3], con->pos[5], con->pos[4]);
			if (a >= 0 && b >= 0 && a != b) { printf("%s[%d,%d]", first?"":",", a, b); first = false; }
		}
	}
	printf("]\n}\n");
}

// Dump the area visibility graph (space-syntax VGA input): cluster polys into
// room-scale areas, then test mutual line-of-sight between area centers via the
// hull-0 tracer (eye height). Consumed by tools/vga.py for integration /
// intelligibility. Requires the world tracer to be active.
static void emit_vga_graph(nav_mesh_poly_record_t *recs, int rc)
{
	const float CELL = 256.0f, ZCELL = 128.0f, EYE = 40.0f;
	std::unordered_map<long long, int> cell2area;
	std::vector<double> ax, ay, az, aarea;
	std::vector<int> acount;
	auto key = [&](int i) -> long long {
		long long cx = (long long)llround(recs[i].center[0] / CELL);
		long long cy = (long long)llround(recs[i].center[1] / CELL);
		long long cz = (long long)llround(recs[i].center[2] / ZCELL);
		return ((cx & 0x1FFFFF) << 42) | ((cy & 0x1FFFFF) << 21) | (cz & 0x1FFFFF);
	};
	for (int i = 0; i < rc; i++) {
		long long k = key(i);
		auto it = cell2area.find(k);
		int a;
		if (it == cell2area.end()) {
			a = (int)ax.size(); cell2area[k] = a;
			ax.push_back(0); ay.push_back(0); az.push_back(0); aarea.push_back(0); acount.push_back(0);
		} else a = it->second;
		ax[a] += recs[i].center[0]; ay[a] += recs[i].center[1]; az[a] += recs[i].center[2];
		aarea[a] += poly_area(recs[i]); acount[a]++;
	}
	int A = (int)ax.size();
	for (int a = 0; a < A; a++) { ax[a] /= acount[a]; ay[a] /= acount[a]; az[a] /= acount[a]; }

	printf("{\n  \"areas\": [");
	for (int a = 0; a < A; a++)
		printf("%s[%.1f,%.1f,%.1f,%.0f]", a?",":"", ax[a], ay[a], az[a], aarea[a]);
	printf("],\n  \"vis_edges\": [");
	bool first = true;
	vec3_t zero = {0,0,0};
	for (int a = 0; a < A; a++) {
		vec3_t ea = {(float)ax[a], (float)ay[a], (float)az[a] + EYE};
		for (int b = a + 1; b < A; b++) {
			vec3_t eb = {(float)ax[b], (float)ay[b], (float)az[b] + EYE};
			trace_t tr = SV_Move(ea, zero, zero, eb, MOVE_NOMONSTERS, nullptr);
			if (tr.fraction >= 0.99f && !tr.startsolid) {
				printf("%s[%d,%d]", first?"":",", a, b); first = false;
			}
		}
	}
	printf("]\n}\n");
}

int main(int argc, char **argv)
{
	std::vector<float> verts;
	std::vector<int> tris;
	std::vector<QueryPoint> points;
	std::vector<nav_off_mesh_link_t> links;
	char err[256] = {0};
	model_t *world = nullptr; // non-null => BSP mode (link callback active)
	bool flow_mode = false, vga_mode = false;
	for (int i = 1; i < argc; i++) {
		if (!strcmp(argv[i], "-flow")) flow_mode = true;
		if (!strcmp(argv[i], "-vga")) vga_mode = true;
	}

	if (argc > 1 && ends_with(argv[1], ".bsp")) {
		world = load_bsp_scene(argv[1], verts, tris, points, links, err, sizeof(err));
		if (!world) {
			fprintf(stderr, "navcheck: %s\n", err);
			return 2;
		}
	} else {
		FILE *in = stdin;
		if (argc > 1) {
			in = fopen(argv[1], "r");
			if (!in) { fprintf(stderr, "navcheck: cannot open %s\n", argv[1]); return 2; }
		}
		if (!read_input(in, verts, tris, points, links, err, sizeof(err))) {
			fprintf(stderr, "navcheck: input error: %s\n", err);
			return 2;
		}
		if (in != stdin) fclose(in);
	}

	nav_mesh_build_config_t cfg;
	default_config(&cfg);
	nav_mesh_summary_t summary;
	err[0] = 0;

	auto build = [&]() -> nav_mesh_runtime_t * {
		return nav_mesh_build(
			verts.data(), (int)(verts.size()/3),
			tris.data(), (int)(tris.size()/3),
			&cfg, links.empty() ? nullptr : links.data(), (int)links.size(),
			&summary, world ? navcheck_link_callback : nullptr, nullptr,
			err, sizeof(err));
	};
	auto append_links = [&](nav_off_mesh_link_t *arr, int n) {
		for (int i = 0; i < n; i++) links.push_back(arr[i]);
	};

	nav_mesh_runtime_t *nav = build();

	// Multi-pass connectivity (BSP mode only — the validator needs the tracer):
	// (2) reconnect orphan clusters via hull-validated jumps, (3) directed
	// one-way links. Each pass augments off-mesh links and rebuilds, matching
	// FrikBotNex's Nav_BuildForMap.
	if (nav && world) {
		nav_off_mesh_link_t *extra = nullptr;
		int n = nav_mesh_compute_orphan_jumps(nav, navcheck_link_validate, nullptr, &extra);
		if (n > 0) { append_links(extra, n); nav_mesh_destroy(nav); nav = build(); }
		free(extra);

		if (nav) {
			extra = nullptr;
			n = nav_mesh_compute_directed_links(nav, navcheck_link_validate, nullptr, &extra);
			if (n > 0) { append_links(extra, n); nav_mesh_destroy(nav); nav = build(); }
			free(extra);
		}
		// VGA needs the tracer (line-of-sight via hull 0) — keep world alive.
		if (!vga_mode) { qents_free(); qworld_free(world); qworld_set_active(nullptr); world = nullptr; }
	}

	if (!nav) {
		printf("{\"ok\":false,\"error\":\"build failed: %s\"}\n", err);
		return 1;
	}

	// Islands + coverage.
	nav_mesh_poly_record_t *recs = nullptr;
	int rec_count = 0;
	nav_mesh_collect_polys(nav, &recs, &rec_count, err, sizeof(err));

	if (vga_mode) {
		emit_vga_graph(recs, rec_count);
		if (world) { qents_free(); qworld_free(world); qworld_set_active(nullptr); }
		nav_mesh_free_poly_records(recs);
		nav_mesh_destroy(nav);
		return 0;
	}

	if (flow_mode) {
		emit_flow_graph(nav, recs, rec_count);
		nav_mesh_free_poly_records(recs);
		nav_mesh_destroy(nav);
		return 0;
	}

	std::unordered_map<unsigned long long, int> ref2island;
	std::vector<int> island_polys;
	int num_islands = label_islands(recs, rec_count, ref2island, island_polys);

	std::vector<float> island_area(num_islands, 0.0f);
	float total_area = 0.0f;
	for (int i = 0; i < rec_count; i++) {
		float a = poly_area(recs[i]);
		island_area[ref2island[recs[i].poly_ref]] += a;
		total_area += a;
	}
	int largest = 0;
	for (int i = 1; i < num_islands; i++)
		if (island_area[i] > island_area[largest]) largest = i;

	// Snap each query point and record its island.
	for (auto &p : points) {
		nav_mesh_nearest_result_t nr;
		if (nav_mesh_find_nearest(nav, p.pos, &nr, err, sizeof(err)) && nr.found) {
			p.snapped = true;
			float dx = nr.nearest_point[0]-p.pos[0];
			float dy = nr.nearest_point[1]-p.pos[1];
			float dz = nr.nearest_point[2]-p.pos[2];
			p.snap_dist = sqrtf(dx*dx+dy*dy+dz*dz);
			auto it = ref2island.find(nr.poly_ref);
			p.island = (it != ref2island.end()) ? it->second : -1;
		}
	}

	// Reference = first label beginning with "spawn", else first point.
	int ref = -1;
	for (size_t i = 0; i < points.size(); i++)
		if (points[i].snapped && points[i].label.rfind("spawn", 0) == 0) { ref = (int)i; break; }
	if (ref < 0)
		for (size_t i = 0; i < points.size(); i++)
			if (points[i].snapped) { ref = (int)i; break; }

	// Reachability from reference.
	int reachable_count = 0, unreachable_count = 0;
	if (ref >= 0) {
		for (size_t i = 0; i < points.size(); i++) {
			if (!points[i].snapped) continue;
			if ((int)i == ref) { points[i].reachable = true; reachable_count++; continue; }
			nav_mesh_path_result_t pr;
			if (nav_mesh_find_path(nav, points[ref].pos, points[i].pos, &pr, err, sizeof(err)) && pr.found) {
				points[i].reachable = true;
				reachable_count++;
			} else {
				unreachable_count++;
			}
		}
	}

	// --- JSON output ---
	printf("{\n");
	printf("  \"ok\": true,\n");
	printf("  \"input\": {\"vertices\": %d, \"triangles\": %d, \"off_mesh_links\": %d},\n",
		(int)(verts.size()/3), (int)(tris.size()/3), (int)links.size());
	printf("  \"navmesh\": {\"polys\": %d, \"islands\": %d, \"largest_island_area_frac\": %.4f},\n",
		rec_count, num_islands, total_area > 0 ? island_area[largest]/total_area : 0.0f);
	printf("  \"islands\": [");
	for (int i = 0; i < num_islands; i++)
		printf("%s{\"id\":%d,\"polys\":%d,\"area\":%.0f}", i?",":"", i, island_polys[i], island_area[i]);
	printf("],\n");
	printf("  \"reference\": %s,\n", ref >= 0 ? ("\"" + points[ref].label + "\"").c_str() : "null");
	printf("  \"reachable\": %d, \"unreachable\": %d, \"off_navmesh\": %d,\n",
		reachable_count, unreachable_count, (int)points.size() - reachable_count - unreachable_count);
	printf("  \"points\": [\n");
	for (size_t i = 0; i < points.size(); i++) {
		const QueryPoint &p = points[i];
		printf("    {\"label\":\"%s\",\"snapped\":%s,\"snap_dist\":%.1f,\"island\":%d,\"reachable\":%s}%s\n",
			p.label.c_str(), p.snapped?"true":"false", p.snap_dist, p.island,
			p.reachable?"true":"false", i+1<points.size()?",":"");
	}
	printf("  ]\n}\n");

	nav_mesh_free_poly_records(recs);
	nav_mesh_destroy(nav);
	return 0;
}
