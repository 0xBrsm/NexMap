// nav_ents — entity-driven geometry + off-mesh links.
// nav_is_brush_entity / nav_extract_bsp / nav_collect_* are lifted VERBATIM
// from FrikBotNex nav_bot.cpp (fix/wp-mesh), run against the qents edict shim
// + qworld submodels instead of the live server. Resync from there.
#ifndef NAV_ENTS_H
#define NAV_ENTS_H

#include "nav_mesh.h"
#include "qworld.h"

#ifdef __cplusplus
extern "C" {
#endif

// Polygonize world clip hull 1 + static brush-entity hulls (thin door floors,
// func_wall, gates, trains) into a triangle soup.
int navcheck_extract_bsp(model_t *worldmodel,
	float **out_verts, int *out_vert_count, int **out_tris, int *out_tri_count);

// Entity off-mesh link collectors (return count, malloc the array).
int navcheck_collect_teleporters(nav_off_mesh_link_t **out_links);
int navcheck_collect_platform_links(nav_off_mesh_link_t **out_links);
int navcheck_collect_train_links(nav_off_mesh_link_t **out_links);

#ifdef __cplusplus
}
#endif

#endif
