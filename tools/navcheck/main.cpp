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

#include <cstdio>
#include <cstdlib>
#include <cstring>
#include <cmath>
#include <vector>
#include <string>
#include <unordered_map>
#include <queue>

// Quake-tuned Recast params (from FrikBotNex nav_bot.cpp — agent radius 16,
// height 56, step 18; cell_size = radius/4 for indoor detail).
static void default_config(nav_mesh_build_config_t *c)
{
	c->cell_size = 4.0f;
	c->cell_height = 2.0f;
	c->walkable_slope_angle = 45.0f;
	c->walkable_height = 56.0f;
	c->walkable_climb = 18.0f;
	c->walkable_radius = 16.0f;
	c->max_edge_len = 192.0f;
	c->max_simplification_error = 1.3f;
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

int main(int argc, char **argv)
{
	FILE *in = stdin;
	if (argc > 1) {
		in = fopen(argv[1], "r");
		if (!in) { fprintf(stderr, "navcheck: cannot open %s\n", argv[1]); return 2; }
	}

	std::vector<float> verts;
	std::vector<int> tris;
	std::vector<QueryPoint> points;
	std::vector<nav_off_mesh_link_t> links;
	char err[256] = {0};
	if (!read_input(in, verts, tris, points, links, err, sizeof(err))) {
		fprintf(stderr, "navcheck: input error: %s\n", err);
		return 2;
	}
	if (in != stdin) fclose(in);

	nav_mesh_build_config_t cfg;
	default_config(&cfg);
	nav_mesh_summary_t summary;
	err[0] = 0;
	nav_mesh_runtime_t *nav = nav_mesh_build(
		verts.data(), (int)(verts.size()/3),
		tris.data(), (int)(tris.size()/3),
		&cfg, links.empty() ? nullptr : links.data(), (int)links.size(),
		&summary, nullptr, nullptr, err, sizeof(err));
	if (!nav) {
		printf("{\"ok\":false,\"error\":\"build failed: %s\"}\n", err);
		return 1;
	}

	// Islands + coverage.
	nav_mesh_poly_record_t *recs = nullptr;
	int rec_count = 0;
	nav_mesh_collect_polys(nav, &recs, &rec_count, err, sizeof(err));
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
