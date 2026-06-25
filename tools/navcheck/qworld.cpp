// qworld — standalone Quake world collision. See qworld.h.
// Collision math adapted from GPL Quake (WinQuake world.c/model.c, id Software).

#include "qworld.h"

#include <cstdio>
#include <cstdlib>
#include <cstring>

vec3_t vec3_origin = {0, 0, 0};
qcvar_t sv_gravity = {800.0f};
qcvar_t sv_maxspeed = {320.0f};

#define DIST_EPSILON 0.03125f

enum { LUMP_PLANES = 1, LUMP_NODES = 5, LUMP_CLIPNODES = 9,
       LUMP_LEAFS = 10, LUMP_MODELS = 14 };

// Owned BSP state (shared by all submodels).
static mplane_t    *g_planes = nullptr;
static dclipnode_t *g_clipnodes = nullptr;  // hulls 1/2
static dclipnode_t *g_hull0nodes = nullptr; // hull 0
static char        *g_entities = nullptr;
static model_t     *g_models = nullptr;
static int          g_num_models = 0;
static model_t     *g_active = nullptr;

namespace {
struct Buf { const unsigned char *p; int len; };
static int rd_i32(const unsigned char *b) {
	return (int)((unsigned)b[0] | ((unsigned)b[1] << 8) | ((unsigned)b[2] << 16) | ((unsigned)b[3] << 24));
}
static short rd_i16(const unsigned char *b) {
	return (short)((unsigned short)(b[0] | (b[1] << 8)));
}
static float rd_f32(const unsigned char *b) {
	int i = rd_i32(b); float f; memcpy(&f, &i, 4); return f;
}
} // namespace

model_t *qworld_load(const char *bsp_path, char *err, int errsz)
{
	FILE *f = fopen(bsp_path, "rb");
	if (!f) { snprintf(err, errsz, "cannot open %s", bsp_path); return nullptr; }
	fseek(f, 0, SEEK_END); long sz = ftell(f); fseek(f, 0, SEEK_SET);
	unsigned char *data = (unsigned char *)malloc(sz);
	if (fread(data, 1, sz, f) != (size_t)sz) { fclose(f); free(data); snprintf(err, errsz, "short read"); return nullptr; }
	fclose(f);

	// BSP29 (version int 29) and BSP2 (magic "BSP2") share the lump layout;
	// BSP2 widens node/leaf/clipnode children to int32 + float bboxes.
	unsigned magic = (unsigned)rd_i32(data);
	bool bsp2 = (magic == 0x32505342u); // 'B','S','P','2'
	if (!bsp2 && magic != 29) { free(data); snprintf(err, errsz, "unsupported BSP magic %u", magic); return nullptr; }
	const int clip_stride = bsp2 ? 12 : 8;
	const int node_stride = bsp2 ? 44 : 24;
	const int leaf_stride = bsp2 ? 44 : 28;
	const int child_w = bsp2 ? 4 : 2;
	auto child = [&](const unsigned char *p) -> int { return bsp2 ? rd_i32(p) : (int)rd_i16(p); };
	auto lump = [&](int i) -> Buf {
		return Buf{ data + rd_i32(data + 4 + i * 8), rd_i32(data + 8 + i * 8) };
	};

	// Planes.
	Buf lp = lump(LUMP_PLANES);
	int nplanes = lp.len / 20;
	g_planes = (mplane_t *)calloc(nplanes > 0 ? nplanes : 1, sizeof(mplane_t));
	for (int i = 0; i < nplanes; i++) {
		const unsigned char *in = lp.p + i * 20;
		for (int k = 0; k < 3; k++) g_planes[i].normal[k] = rd_f32(in + k * 4);
		g_planes[i].dist = rd_f32(in + 12);
		g_planes[i].type = rd_i32(in + 16);
	}

	// Clipnodes (hulls 1/2): planenum:int + children[2] (int16 BSP29 / int32 BSP2).
	Buf lc = lump(LUMP_CLIPNODES);
	int nclip = lc.len / clip_stride;
	g_clipnodes = (dclipnode_t *)calloc(nclip > 0 ? nclip : 1, sizeof(dclipnode_t));
	for (int i = 0; i < nclip; i++) {
		const unsigned char *in = lc.p + i * clip_stride;
		g_clipnodes[i].planenum = rd_i32(in);
		g_clipnodes[i].children[0] = child(in + 4);
		g_clipnodes[i].children[1] = child(in + 4 + child_w);
	}

	// Nodes + leafs -> hull 0. leaf.contents is int32 at offset 0 in both formats.
	Buf ll = lump(LUMP_LEAFS); int nleafs = ll.len / leaf_stride;
	Buf ln = lump(LUMP_NODES); int nnodes = ln.len / node_stride;
	g_hull0nodes = (dclipnode_t *)calloc(nnodes > 0 ? nnodes : 1, sizeof(dclipnode_t));
	for (int i = 0; i < nnodes; i++) {
		const unsigned char *in = ln.p + i * node_stride;
		g_hull0nodes[i].planenum = rd_i32(in);
		for (int j = 0; j < 2; j++) {
			int c = child(in + 4 + j * child_w);
			if (c >= 0) g_hull0nodes[i].children[j] = c;
			else {
				int leafnum = -1 - c;
				g_hull0nodes[i].children[j] = (leafnum >= 0 && leafnum < nleafs)
					? rd_i32(ll.p + leafnum * leaf_stride) : CONTENTS_SOLID;
			}
		}
	}

	// Per-submodel: hulls keyed on its own headnode, plus bbox/origin.
	static const float cmins[3][3] = {{0,0,0},{-16,-16,-24},{-32,-32,-24}};
	static const float cmaxs[3][3] = {{0,0,0},{16,16,32},{32,32,64}};
	Buf lm = lump(LUMP_MODELS);
	g_num_models = lm.len / 64;
	g_models = (model_t *)calloc(g_num_models > 0 ? g_num_models : 1, sizeof(model_t));
	for (int mi = 0; mi < g_num_models; mi++) {
		const unsigned char *in = lm.p + mi * 64;
		model_t *m = &g_models[mi];
		for (int k = 0; k < 3; k++) {
			m->mins[k]   = rd_f32(in + k * 4);
			m->maxs[k]   = rd_f32(in + 12 + k * 4);
			m->origin[k] = rd_f32(in + 24 + k * 4);
		}
		int headnode[4];
		for (int k = 0; k < 4; k++) headnode[k] = rd_i32(in + 36 + k * 4);
		hull_t *h0 = &m->hulls[0];
		h0->clipnodes = g_hull0nodes; h0->planes = g_planes;
		h0->firstclipnode = headnode[0]; h0->lastclipnode = nnodes - 1;
		for (int hi = 1; hi <= 2; hi++) {
			hull_t *h = &m->hulls[hi];
			h->clipnodes = g_clipnodes; h->planes = g_planes;
			h->firstclipnode = headnode[hi]; h->lastclipnode = nclip - 1;
			for (int k = 0; k < 3; k++) { h->clip_mins[k] = cmins[hi][k]; h->clip_maxs[k] = cmaxs[hi][k]; }
		}
	}

	// Entity lump.
	Buf le = lump(0);
	g_entities = (char *)malloc(le.len + 1);
	memcpy(g_entities, le.p, le.len);
	g_entities[le.len] = '\0';

	free(data);
	return g_num_models > 0 ? &g_models[0] : nullptr;
}

void qworld_free(model_t *world)
{
	(void)world;
	free(g_planes); free(g_clipnodes); free(g_hull0nodes);
	free(g_entities); free(g_models);
	g_planes = nullptr; g_clipnodes = nullptr; g_hull0nodes = nullptr;
	g_entities = nullptr; g_models = nullptr; g_num_models = 0;
}

int qworld_num_models(void) { return g_num_models; }
model_t *qworld_model(int i) { return (i >= 0 && i < g_num_models) ? &g_models[i] : nullptr; }
const char *qworld_entities(void) { return g_entities ? g_entities : ""; }
void qworld_set_active(model_t *m) { g_active = m; }

int SV_HullPointContents(hull_t *hull, int num, const vec3_t p)
{
	while (num >= 0) {
		if (num < hull->firstclipnode || num > hull->lastclipnode) return CONTENTS_SOLID;
		dclipnode_t *node = hull->clipnodes + num;
		mplane_t *plane = hull->planes + node->planenum;
		float d = (plane->type < 3) ? p[plane->type] - plane->dist
		                            : DotProduct(plane->normal, p) - plane->dist;
		num = (d < 0) ? node->children[1] : node->children[0];
	}
	return num;
}

int SV_PointContents(const vec3_t p)
{
	if (!g_active) return CONTENTS_EMPTY;
	int cont = SV_HullPointContents(&g_active->hulls[0], g_active->hulls[0].firstclipnode, p);
	if (cont <= CONTENTS_CURRENT_0 && cont >= CONTENTS_CURRENT_DOWN) cont = CONTENTS_WATER;
	return cont;
}

static bool RecursiveHullCheck(hull_t *hull, int num, float p1f, float p2f,
	const vec3_t p1, const vec3_t p2, trace_t *trace)
{
	if (num < 0) {
		if (num != CONTENTS_SOLID) {
			trace->allsolid = 0;
			if (num == CONTENTS_EMPTY) trace->inopen = 1; else trace->inwater = 1;
		} else trace->startsolid = 1;
		return true;
	}
	if (num < hull->firstclipnode || num > hull->lastclipnode) return true;

	dclipnode_t *node = hull->clipnodes + num;
	mplane_t *plane = hull->planes + node->planenum;
	float t1, t2;
	if (plane->type < 3) { t1 = p1[plane->type] - plane->dist; t2 = p2[plane->type] - plane->dist; }
	else { t1 = DotProduct(plane->normal, p1) - plane->dist; t2 = DotProduct(plane->normal, p2) - plane->dist; }
	if (t1 >= 0 && t2 >= 0) return RecursiveHullCheck(hull, node->children[0], p1f, p2f, p1, p2, trace);
	if (t1 < 0 && t2 < 0) return RecursiveHullCheck(hull, node->children[1], p1f, p2f, p1, p2, trace);

	float frac = (t1 < 0) ? (t1 + DIST_EPSILON) / (t1 - t2) : (t1 - DIST_EPSILON) / (t1 - t2);
	if (frac < 0) frac = 0; if (frac > 1) frac = 1;
	float midf = p1f + (p2f - p1f) * frac;
	vec3_t mid;
	for (int i = 0; i < 3; i++) mid[i] = p1[i] + frac * (p2[i] - p1[i]);
	int side = (t1 < 0);

	if (!RecursiveHullCheck(hull, node->children[side], p1f, midf, p1, mid, trace)) return false;
	if (SV_HullPointContents(hull, node->children[side ^ 1], mid) != CONTENTS_SOLID)
		return RecursiveHullCheck(hull, node->children[side ^ 1], midf, p2f, mid, p2, trace);
	if (trace->allsolid) return false;

	if (!side) { VectorCopy(plane->normal, trace->plane.normal); trace->plane.dist = plane->dist; }
	else { VectorSubtract(vec3_origin, plane->normal, trace->plane.normal); trace->plane.dist = -plane->dist; }

	while (SV_HullPointContents(hull, hull->firstclipnode, mid) == CONTENTS_SOLID) {
		frac -= 0.1f;
		if (frac < 0) { trace->fraction = midf; VectorCopy(mid, trace->endpos); return false; }
		midf = p1f + (p2f - p1f) * frac;
		for (int i = 0; i < 3; i++) mid[i] = p1[i] + frac * (p2[i] - p1[i]);
	}
	trace->fraction = midf;
	VectorCopy(mid, trace->endpos);
	return false;
}

trace_t SV_Move(const vec3_t start, const vec3_t mins, const vec3_t maxs,
	const vec3_t end, int type, edict_t *passedict)
{
	(void)type; (void)passedict;
	trace_t trace;
	memset(&trace, 0, sizeof(trace));
	trace.fraction = 1; trace.allsolid = 1;
	VectorCopy(end, trace.endpos);
	if (!g_active) return trace;

	float size = maxs[0] - mins[0];
	hull_t *hull = (size < 3) ? &g_active->hulls[0]
	             : (size <= 32) ? &g_active->hulls[1] : &g_active->hulls[2];
	vec3_t offset, start_l, end_l;
	VectorSubtract(hull->clip_mins, mins, offset);
	VectorSubtract(start, offset, start_l);
	VectorSubtract(end, offset, end_l);

	RecursiveHullCheck(hull, hull->firstclipnode, 0, 1, start_l, end_l, &trace);

	if (trace.fraction == 1) { VectorCopy(end, trace.endpos); }
	else { VectorAdd(trace.endpos, offset, trace.endpos); }
	return trace;
}
