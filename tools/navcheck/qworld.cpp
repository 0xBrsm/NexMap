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

// BSP29 lump indices.
enum { LUMP_PLANES = 1, LUMP_NODES = 5, LUMP_CLIPNODES = 9,
       LUMP_LEAFS = 10, LUMP_MODELS = 14, NUM_LUMPS = 15 };

static model_t *g_active = nullptr;

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
	fseek(f, 0, SEEK_END);
	long sz = ftell(f);
	fseek(f, 0, SEEK_SET);
	unsigned char *data = (unsigned char *)malloc(sz);
	if (fread(data, 1, sz, f) != (size_t)sz) { fclose(f); free(data); snprintf(err, errsz, "short read"); return nullptr; }
	fclose(f);

	int version = rd_i32(data);
	if (version != 29) { free(data); snprintf(err, errsz, "unsupported BSP version %d", version); return nullptr; }

	auto lump = [&](int i) -> Buf {
		int off = rd_i32(data + 4 + i * 8);
		int len = rd_i32(data + 8 + i * 8);
		return Buf{ data + off, len };
	};

	model_t *m = (model_t *)calloc(1, sizeof(model_t));

	// Planes (20 bytes: normal[3] dist type).
	Buf lp = lump(LUMP_PLANES);
	int nplanes = lp.len / 20;
	m->planes = (mplane_t *)calloc(nplanes, sizeof(mplane_t));
	for (int i = 0; i < nplanes; i++) {
		const unsigned char *in = lp.p + i * 20;
		for (int k = 0; k < 3; k++) m->planes[i].normal[k] = rd_f32(in + k * 4);
		m->planes[i].dist = rd_f32(in + 12);
		m->planes[i].type = rd_i32(in + 16);
	}

	// Clipnodes (8 bytes: planenum:int children[2]:short) -> hulls 1/2.
	Buf lc = lump(LUMP_CLIPNODES);
	int nclip = lc.len / 8;
	m->clipnodes = (dclipnode_t *)calloc(nclip > 0 ? nclip : 1, sizeof(dclipnode_t));
	for (int i = 0; i < nclip; i++) {
		const unsigned char *in = lc.p + i * 8;
		m->clipnodes[i].planenum = rd_i32(in);
		m->clipnodes[i].children[0] = rd_i16(in + 4); // sign-extended
		m->clipnodes[i].children[1] = rd_i16(in + 6);
	}

	// Leafs (28 bytes: contents:int ...).
	Buf ll = lump(LUMP_LEAFS);
	int nleafs = ll.len / 28;
	// Nodes (24 bytes: planenum:int children[2]:short ...) -> build hull 0.
	Buf ln = lump(LUMP_NODES);
	int nnodes = ln.len / 24;
	m->hull0nodes = (dclipnode_t *)calloc(nnodes > 0 ? nnodes : 1, sizeof(dclipnode_t));
	for (int i = 0; i < nnodes; i++) {
		const unsigned char *in = ln.p + i * 24;
		m->hull0nodes[i].planenum = rd_i32(in);
		for (int j = 0; j < 2; j++) {
			short c = rd_i16(in + 4 + j * 2);
			if (c >= 0) {
				m->hull0nodes[i].children[j] = c; // node index
			} else {
				int leafnum = -1 - c;
				int contents = (leafnum >= 0 && leafnum < nleafs)
					? rd_i32(ll.p + leafnum * 28) : CONTENTS_SOLID;
				m->hull0nodes[i].children[j] = contents;
			}
		}
	}

	// Model 0 = world (64 bytes: bbox[6] origin[3] headnode[4] ...).
	Buf lm = lump(LUMP_MODELS);
	const unsigned char *m0 = lm.p; // first model
	for (int k = 0; k < 3; k++) {
		m->mins[k] = rd_f32(m0 + k * 4);
		m->maxs[k] = rd_f32(m0 + 12 + k * 4);
	}
	int headnode[4];
	for (int k = 0; k < 4; k++) headnode[k] = rd_i32(m0 + 36 + k * 4);

	// Capture all submodel bboxes (for brush-entity volumes like teleporters).
	m->num_models = lm.len / 64;
	m->model_mins = (vec3_t *)calloc(m->num_models > 0 ? m->num_models : 1, sizeof(vec3_t));
	m->model_maxs = (vec3_t *)calloc(m->num_models > 0 ? m->num_models : 1, sizeof(vec3_t));
	for (int i = 0; i < m->num_models; i++) {
		const unsigned char *mi = lm.p + i * 64;
		for (int k = 0; k < 3; k++) {
			m->model_mins[i][k] = rd_f32(mi + k * 4);
			m->model_maxs[i][k] = rd_f32(mi + 12 + k * 4);
		}
	}

	// Entity lump (0) as a NUL-terminated string.
	Buf le = lump(0);
	m->entities = (char *)malloc(le.len + 1);
	memcpy(m->entities, le.p, le.len);
	m->entities[le.len] = '\0';

	// hull 0 (point traces): nodes-as-clipnodes.
	hull_t *h = &m->hulls[0];
	h->clipnodes = m->hull0nodes; h->planes = m->planes;
	h->firstclipnode = headnode[0]; h->lastclipnode = nnodes - 1;

	// hulls 1 & 2 (box traces): clipnodes, pre-expanded by qbsp.
	static const float cmins1[3] = {-16,-16,-24}, cmaxs1[3] = {16,16,32};
	static const float cmins2[3] = {-32,-32,-24}, cmaxs2[3] = {32,32,64};
	for (int hi = 1; hi <= 2; hi++) {
		h = &m->hulls[hi];
		h->clipnodes = m->clipnodes; h->planes = m->planes;
		h->firstclipnode = headnode[hi]; h->lastclipnode = nclip - 1;
		const float *cm = hi == 1 ? cmins1 : cmins2;
		const float *cx = hi == 1 ? cmaxs1 : cmaxs2;
		for (int k = 0; k < 3; k++) { h->clip_mins[k] = cm[k]; h->clip_maxs[k] = cx[k]; }
	}

	free(data);
	return m;
}

void qworld_free(model_t *m)
{
	if (!m) return;
	free(m->planes); free(m->clipnodes); free(m->hull0nodes);
	free(m->entities); free(m->model_mins); free(m->model_maxs); free(m);
}

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
	// check for empty
	if (num < 0) {
		if (num != CONTENTS_SOLID) {
			trace->allsolid = 0;
			if (num == CONTENTS_EMPTY) trace->inopen = 1; else trace->inwater = 1;
		} else {
			trace->startsolid = 1;
		}
		return true;
	}
	if (num < hull->firstclipnode || num > hull->lastclipnode) return true; // tolerate bad refs

	dclipnode_t *node = hull->clipnodes + num;
	mplane_t *plane = hull->planes + node->planenum;
	float t1, t2;
	if (plane->type < 3) {
		t1 = p1[plane->type] - plane->dist;
		t2 = p2[plane->type] - plane->dist;
	} else {
		t1 = DotProduct(plane->normal, p1) - plane->dist;
		t2 = DotProduct(plane->normal, p2) - plane->dist;
	}
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

	if (trace->allsolid) return false; // never got out of solid

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
	trace.fraction = 1;
	trace.allsolid = 1;
	VectorCopy(end, trace.endpos);
	if (!g_active) return trace;

	// pick hull by box size (SV_HullForEntity, world model)
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
