// qworld — minimal standalone Quake world collision for navcheck.
//
// Loads a BSP29 file's clip hulls (hull 0 from nodes via Mod_MakeHull0, hulls
// 1/2 from the clipnodes lump) for every submodel, and provides SV_Move /
// SV_PointContents so the ported nav_hull (hull-1 polygonization) and the
// entity-link collectors can run offline, matching what FrikBot's bots
// collide against in-engine.
//
// Collision routines (SV_HullPointContents / SV_RecursiveHullCheck) are
// adapted from the GPL Quake source (WinQuake world.c / model.c, id Software).

#ifndef QWORLD_H
#define QWORLD_H

#ifdef __cplusplus
extern "C" {
#endif

typedef float vec3_t[3];

#define MAX_MAP_HULLS 4

// Quake leaf contents (negative leaf values in the hull trees).
#define CONTENTS_EMPTY  -1
#define CONTENTS_SOLID  -2
#define CONTENTS_WATER  -3
#define CONTENTS_SLIME  -4
#define CONTENTS_LAVA   -5
#define CONTENTS_SKY    -6
#define CONTENTS_CURRENT_0    -9
#define CONTENTS_CURRENT_DOWN -14

#define MOVE_NORMAL    0
#define MOVE_NOMONSTERS 1
#define MOVE_MISSILE   2

typedef struct mplane_s {
	vec3_t normal;
	float  dist;
	int    type;       // 0,1,2 = axial; 3 = non-axial
} mplane_t;

// In-memory clip node: children hold a node index (>=0) or contents (<0).
typedef struct dclipnode_s {
	int planenum;
	int children[2];
} dclipnode_t;

typedef struct hull_s {
	dclipnode_t *clipnodes;
	mplane_t    *planes;
	int          firstclipnode;
	int          lastclipnode;
	vec3_t       clip_mins;
	vec3_t       clip_maxs;
} hull_t;

typedef struct model_s {
	vec3_t mins, maxs, origin;
	hull_t hulls[MAX_MAP_HULLS];
} model_t;

typedef struct edict_s edict_t; // opaque to qworld; defined in qents.h

typedef struct trace_s {
	int   allsolid;
	int   startsolid;
	int   inopen, inwater;
	float fraction;
	vec3_t endpos;
	struct { vec3_t normal; float dist; } plane;
	edict_t *ent;
} trace_t;

// cvar-like shims so ported code can read sv_gravity.value etc.
typedef struct { float value; } qcvar_t;
extern qcvar_t sv_gravity;
extern qcvar_t sv_maxspeed;

#define DotProduct(a,b) ((a)[0]*(b)[0]+(a)[1]*(b)[1]+(a)[2]*(b)[2])
#define VectorCopy(a,b) {(b)[0]=(a)[0];(b)[1]=(a)[1];(b)[2]=(a)[2];}
#define VectorSubtract(a,b,c) {(c)[0]=(a)[0]-(b)[0];(c)[1]=(a)[1]-(b)[1];(c)[2]=(a)[2]-(b)[2];}
#define VectorAdd(a,b,c) {(c)[0]=(a)[0]+(b)[0];(c)[1]=(a)[1]+(b)[1];(c)[2]=(a)[2]+(b)[2];}
extern vec3_t vec3_origin;

// --- API ---
model_t *qworld_load(const char *bsp_path, char *err, int errsz); // returns world (submodel 0)
void     qworld_free(model_t *world);
int      qworld_num_models(void);
model_t *qworld_model(int i);              // submodel i (i==0 is world)
const char *qworld_entities(void);         // entity lump string

void     qworld_set_active(model_t *m);    // world model for SV_Move/SV_PointContents

int      SV_HullPointContents(hull_t *hull, int num, const vec3_t p);
int      SV_PointContents(const vec3_t p);
trace_t  SV_Move(const vec3_t start, const vec3_t mins, const vec3_t maxs,
		const vec3_t end, int type, edict_t *passedict);

#ifdef __cplusplus
}
#endif

#endif // QWORLD_H
