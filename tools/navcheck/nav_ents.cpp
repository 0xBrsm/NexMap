// nav_ents.cpp — entity-driven geometry + off-mesh links. See nav_ents.h.
//
// The functions below are copied VERBATIM from FrikBotNex nav_bot.cpp
// (fix/wp-mesh) — do not edit; resync from there. They run against the qents
// edict shim (sv/EDICT_NUM/pr_strings/GetEdictFieldValue) and qworld submodels
// (sv.models[]) instead of the live Quake server.

#include "nav_mesh.h"
#include "nav_hull.h"
#include "qworld.h"
#include "qents.h"
#include "nav_ents.h"

#include <cmath>
#include <cstdlib>
#include <cstring>
#include <cstdio>

// ===== BEGIN verbatim from FrikBotNex nav_bot.cpp =====
/* Brush entities whose clip hulls belong in the navmesh. */
/* QC spawn code renames brush entities (doors.qc/plats.qc):
   func_door & func_door_secret -> "door", func_plat -> "plat",
   func_train -> "train".  Nav builds after spawn, so match the
   renamed forms.

   Doors are special: thin horizontal slabs (dm2 water covers, dm6
   secret-door platform) are floors bots stand on and must be in the
   mesh.  Tall doors block passages bots path through (they open on
   touch), so those stay out.

   Plats are deliberately excluded: baking the plat body at its
   resting position fragments the surrounding mesh (dm3 lost edges),
   and plat traversal is the off-mesh link system's job. */
#define NAV_DOOR_FLOOR_MAX_THICKNESS 32.0f

static int nav_is_brush_entity(edict_t *e)
{
	char *classname = pr_strings + (int)e->v.classname;
	if (!strcasecmp(classname, "door"))
		return (e->v.absmax[2] - e->v.absmin[2]) <= NAV_DOOR_FLOOR_MAX_THICKNESS;
	return !strncasecmp(classname, "func_wall", 9)
		|| !strncasecmp(classname, "func_episodegate", 16)
		|| !strncasecmp(classname, "func_bossgate", 13)
		|| !strcasecmp(classname, "train");
}

/* Polygonize clip hull 1 of the world plus static brush entities.
   See nav_hull.cpp for why hull geometry instead of render faces. */
static int nav_extract_bsp(model_t *worldmodel,
	float **out_verts, int *out_vert_count,
	int **out_tris, int *out_tri_count)
{
	int i;

	*out_verts = NULL; *out_vert_count = 0;
	*out_tris = NULL;  *out_tri_count = 0;
	if (!worldmodel) return 0;

	nav_hull_begin();
	nav_hull_add_model(worldmodel, NULL);

	for (i = 1; i < sv.num_edicts; i++)
	{
		edict_t *e = EDICT_NUM(i);
		model_t *m;
		if (e->free) continue;
		m = sv.models[(int)e->v.modelindex];
		if (!m || m == worldmodel) continue;
		if (!nav_is_brush_entity(e)) continue;
		nav_hull_add_model(m, e->v.origin);
	}

	return nav_hull_end(out_verts, out_vert_count, out_tris, out_tri_count);
}

/* ---- Teleporter off-mesh links ---- */

static int nav_collect_teleporters(nav_off_mesh_link_t **out_links)
{
	int i, j, count, n;
	nav_off_mesh_link_t *links;

	count = 0;
	for (i = 1; i < sv.num_edicts; i++)
	{
		edict_t *e = EDICT_NUM(i);
		if (!e->free && !strcasecmp(pr_strings + (int)e->v.classname, "trigger_teleport"))
			count++;
	}
	if (count == 0) { *out_links = NULL; return 0; }

	links = (nav_off_mesh_link_t *)calloc(count, sizeof(*links));
	n = 0;
	for (i = 1; i < sv.num_edicts; i++)
	{
		edict_t *src = EDICT_NUM(i);
		const char *tgt;
		if (src->free) continue;
		if (strcasecmp(pr_strings + (int)src->v.classname, "trigger_teleport")) continue;
		tgt = src->v.target ? pr_strings + (int)src->v.target : "";
		if (!tgt[0]) continue;

		for (j = 1; j < sv.num_edicts; j++)
		{
			edict_t *dst = EDICT_NUM(j);
			const char *tn;
			if (dst->free) continue;
			if (strcasecmp(pr_strings + (int)dst->v.classname, "info_teleport_destination"))
				continue;
			tn = dst->v.targetname ? pr_strings + (int)dst->v.targetname : "";
			if (strcmp(tgt, tn)) continue;

			links[n].start[0] = (src->v.absmin[0] + src->v.absmax[0]) * 0.5f;
			links[n].start[1] = (src->v.absmin[1] + src->v.absmax[1]) * 0.5f;
			links[n].start[2] = src->v.absmin[2];
			links[n].end[0] = dst->v.origin[0];
			links[n].end[1] = dst->v.origin[1];
			links[n].end[2] = dst->v.origin[2];
			links[n].radius = 128.0f;
			links[n].bidirectional = 0;
			links[n].link_type = AI_TELELINK;
			links[n].required_speed = 0;
			links[n].height_delta = 0;
			n++;
			break;
		}
	}
	*out_links = links;
	return n;
}

/* ---- Platform link detection ---- */

/* Scan plat entities (func_plat renames itself "plat" at spawn).
   Create bidirectional links between top and bottom standing surfaces.
   Bot rides the platform to traverse. */
static int nav_collect_platform_links(nav_off_mesh_link_t **out_links)
{
	int i, n = 0, cap = 16;
	nav_off_mesh_link_t *links;

	links = (nav_off_mesh_link_t *)calloc(cap, sizeof(*links));
	*out_links = links;

	for (i = 1; i < sv.num_edicts; i++)
	{
		edict_t *e = EDICT_NUM(i);
		eval_t *pos1, *pos2, *spd;
		float top_z, bot_z, speed, travel;
		if (e->free) continue;
		if (strcasecmp(pr_strings + (int)e->v.classname, "plat")) continue;

		/* pos1 = top, pos2 = bottom (QC fields, set by plat spawn code).
		   Link endpoints are where the bot STANDS: brush top surface
		   (pos z + maxs z), which is flush with the floor at each stop. */
		pos1 = GetEdictFieldValue(e, "pos1");
		pos2 = GetEdictFieldValue(e, "pos2");
		if (!pos1 || !pos2) continue;
		top_z = pos1->vector[2] + e->v.maxs[2];
		bot_z = pos2->vector[2] + e->v.maxs[2];

		spd = GetEdictFieldValue(e, "speed");
		speed = (spd && spd->_float > 0) ? spd->_float : 150.0f;
		travel = (top_z - bot_z) / speed;

		if (n >= cap) { cap *= 2; links = (nav_off_mesh_link_t *)realloc(links, cap * sizeof(*links)); }

		/* Center of platform XY */
		links[n].start[0] = (e->v.absmin[0] + e->v.absmax[0]) * 0.5f;
		links[n].start[1] = (e->v.absmin[1] + e->v.absmax[1]) * 0.5f;
		links[n].start[2] = bot_z;
		links[n].end[0] = links[n].start[0];
		links[n].end[1] = links[n].start[1];
		links[n].end[2] = top_z;
		links[n].radius = 64.0f;
		links[n].bidirectional = 1;
		links[n].link_type = AI_PLAT_BOTTOM;
		links[n].height_delta = top_z - bot_z;
		links[n].wait_time = travel;
		links[n].required_speed = 0;
		if (nav_debug_cvar.value)
			Con_Printf("Nav: plat link (%.0f %.0f) z %.0f -> %.0f spd %.0f\n",
				links[n].start[0], links[n].start[1], bot_z, top_z, speed);
		n++;
	}

	*out_links = links;
	return n;
}

/* ---- Train link detection ---- */

/* Scan func_train entities.  Create links between consecutive path_corner
   stops.  Trains follow path_corner chains. */
static int nav_collect_train_links(nav_off_mesh_link_t **out_links)
{
	int i, j, n = 0, cap = 16;
	nav_off_mesh_link_t *links;

	links = (nav_off_mesh_link_t *)calloc(cap, sizeof(*links));
	*out_links = links;

	for (i = 1; i < sv.num_edicts; i++)
	{
		edict_t *e = EDICT_NUM(i);
		const char *tgt;
		if (e->free) continue;
		if (strcasecmp(pr_strings + (int)e->v.classname, "train")) continue;

		/* Walk the path_corner chain */
		tgt = e->v.target ? pr_strings + (int)e->v.target : "";
		if (!tgt[0]) continue;

		/* Find first path_corner */
		for (j = 1; j < sv.num_edicts; j++)
		{
			edict_t *pc = EDICT_NUM(j);
			edict_t *next_pc;
			const char *pcname, *pctgt;
			int k;
			if (pc->free) continue;
			if (strcasecmp(pr_strings + (int)pc->v.classname, "path_corner")) continue;
			pcname = pc->v.targetname ? pr_strings + (int)pc->v.targetname : "";
			if (strcmp(tgt, pcname)) continue;

			/* Found start — follow chain, create links between stops */
			pctgt = pc->v.target ? pr_strings + (int)pc->v.target : "";
			if (!pctgt[0]) break;

			for (k = 1; k < sv.num_edicts; k++)
			{
				next_pc = EDICT_NUM(k);
				const char *nname;
				float dist;
				if (next_pc->free) continue;
				if (strcasecmp(pr_strings + (int)next_pc->v.classname, "path_corner")) continue;
				nname = next_pc->v.targetname ? pr_strings + (int)next_pc->v.targetname : "";
				if (strcmp(pctgt, nname)) continue;

				/* Create link between this path_corner and next.
				   Trains move so their MINS corner sits at the path_corner
				   (func_train_find: origin = corner - mins), so the bot
				   stands at corner + size/2 XY, corner z + size z. */
				if (n >= cap) { cap *= 2; links = (nav_off_mesh_link_t *)realloc(links, cap * sizeof(*links)); }
				links[n].start[0] = pc->v.origin[0] + e->v.size[0] * 0.5f;
				links[n].start[1] = pc->v.origin[1] + e->v.size[1] * 0.5f;
				links[n].start[2] = pc->v.origin[2] + e->v.size[2];
				links[n].end[0] = next_pc->v.origin[0] + e->v.size[0] * 0.5f;
				links[n].end[1] = next_pc->v.origin[1] + e->v.size[1] * 0.5f;
				links[n].end[2] = next_pc->v.origin[2] + e->v.size[2];
				dist = sqrt((links[n].end[0]-links[n].start[0])*(links[n].end[0]-links[n].start[0])
					+ (links[n].end[1]-links[n].start[1])*(links[n].end[1]-links[n].start[1])
					+ (links[n].end[2]-links[n].start[2])*(links[n].end[2]-links[n].start[2]));
				links[n].radius = 64.0f;
				links[n].bidirectional = 0;
				links[n].link_type = AI_RIDE_TRAIN;
				links[n].height_delta = next_pc->v.origin[2] - pc->v.origin[2];
				links[n].wait_time = dist / (100.0f);
				links[n].required_speed = 0;
				n++;
				break;
			}
			break; /* only process first path_corner match */
		}
	}

	*out_links = links;
	return n;
}

// ===== END verbatim =====

extern "C" int navcheck_extract_bsp(model_t *w, float **v, int *vc, int **t, int *tc)
{ return nav_extract_bsp(w, v, vc, t, tc); }
extern "C" int navcheck_collect_teleporters(nav_off_mesh_link_t **l)
{ return nav_collect_teleporters(l); }
extern "C" int navcheck_collect_platform_links(nav_off_mesh_link_t **l)
{ return nav_collect_platform_links(l); }
extern "C" int navcheck_collect_train_links(nav_off_mesh_link_t **l)
{ return nav_collect_train_links(l); }
