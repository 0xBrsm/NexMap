// qents — minimal edict/sv shim backed by the BSP entity lump + qworld
// submodels, so FrikBot's entity geometry/link code (nav_is_brush_entity,
// nav_extract_bsp, nav_collect_*) lifts verbatim with no live server.
//
// Reproduces the bits of QuakeC spawn the nav code depends on: brush-entity
// classname renaming (func_door->door, func_plat->plat, func_train->train)
// and func_plat pos1/pos2 (plats.qc).

#ifndef QENTS_H
#define QENTS_H

#include "qworld.h"

#ifdef __cplusplus
extern "C" {
#endif

typedef struct { float _float; vec3_t vector; } eval_t;

typedef struct {
	int   classname, target, targetname; // byte offsets into pr_strings
	float modelindex;
	vec3_t origin, absmin, absmax, mins, maxs, size;
} entvars_t;

struct edict_s {
	int       free;
	entvars_t v;
	eval_t    pos1, pos2, speed; // precomputed QC fields (func_plat)
	int       has_pos;           // pos1/pos2 valid (is a plat)
};

typedef struct {
	int       num_edicts;
	edict_t  *edicts;
	model_t **models;   // indexed by modelindex
	int       nummodels;
} qsv_t;

extern qsv_t sv;
extern char *pr_strings;
extern qcvar_t nav_debug_cvar;

#define EDICT_NUM(i) (&sv.edicts[i])

eval_t *GetEdictFieldValue(edict_t *e, const char *field);

// Build sv/edicts from the loaded qworld BSP (entity lump + submodels).
void qents_load(void);
void qents_free(void);

#ifdef __cplusplus
}
#endif

// Con_Printf shim (nav debug, gated off by nav_debug_cvar). Variadic no-op.
#ifdef __cplusplus
static inline void Con_Printf(const char *, ...) {}
#endif

#endif // QENTS_H
