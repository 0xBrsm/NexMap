// nav_links — geometric off-mesh link generation (drops/jumps/bridges/rocket).
// Extracted from FrikBotNex nav_bot.cpp (fix/wp-mesh), run against the qworld
// clip-hull tracer instead of the live server.
#ifndef NAV_LINKS_H
#define NAV_LINKS_H

#include "nav_mesh.h"

#ifdef __cplusplus
extern "C" {
#endif

// Matches nav_mesh_link_callback_t; pass to nav_mesh_build.
int navcheck_link_callback(
	const nav_mesh_boundary_edge_t *edges, int edge_count,
	const nav_heightfield_t *hf,
	nav_off_mesh_link_t **out_links,
	void *user_data);

// Matches nav_jump_validate_fn; pass to the orphan/directed link passes.
int navcheck_link_validate(const float *from, const float *to, void *user);

#ifdef __cplusplus
}
#endif

#endif
