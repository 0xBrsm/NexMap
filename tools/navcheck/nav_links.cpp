// nav_links.cpp — geometric off-mesh link generation. See nav_links.h.
//
// The three helpers and nav_link_callback below are copied VERBATIM from
// FrikBotNex's nav_bot.cpp (fix/wp-mesh) — do not edit; resync from there.
// They run against the qworld clip-hull tracer (SV_Move/SV_PointContents)
// instead of the live Quake server. sv_gravity/sv_maxspeed come from qworld.

#include "nav_mesh.h"
#include "nav_links.h"
#include "qworld.h"

#include <cmath>
#include <cstdlib>
#include <cstring>
#include <cstdio>

// Link constants used by the helpers + callback (from nav_bot.cpp).
#define NAV_JUMP_IMPULSE            270.0f
#define NAV_JUMP_HEIGHT_MIN         18.0f
#define NAV_JUMP_HEIGHT_MAX         48.0f
#define NAV_RJ_HEIGHT_MAX          256.0f
#define NAV_RJ_HORIZ_MAX           128.0f
#define NAV_DROP_HEIGHT_MIN          6.0f
#define NAV_DROP_HEIGHT_MAX        192.0f
#define NAV_WATER_DROP_HEIGHT_MAX  400.0f
#define NAV_JUMP_LINK_RADIUS        16.0f
#define NAV_PLAYER_FLOOR_OFFSET     24.0f

// ===== BEGIN verbatim from FrikBotNex nav_bot.cpp =====
static int nav_trace_clear_at_height(const float *start, const float *end, float z, edict_t *passedict)
{
	vec3_t trace_start;
	vec3_t trace_end;
	vec3_t zero = {0, 0, 0};
	trace_t trace;

	VectorCopy(start, trace_start);
	VectorCopy(end, trace_end);
	trace_start[2] = z;
	trace_end[2] = z;
	trace = SV_Move(trace_start, zero, zero, trace_end, MOVE_NOMONSTERS, passedict);
	return !trace.allsolid && !trace.startsolid && trace.fraction >= 1.0f;
}


/* Helper: add a link, growing the array if needed. */
static void nav_link_push(nav_off_mesh_link_t **links, int *n, int *cap,
	const float *start, const float *end, int type, float speed, float dz)
{
	if (*n >= *cap)
	{
		*cap *= 2;
		*links = (nav_off_mesh_link_t *)realloc(*links, (size_t)*cap * sizeof(**links));
	}
	nav_off_mesh_link_t *l = &(*links)[*n];
	l->start[0] = start[0]; l->start[1] = start[1]; l->start[2] = start[2];
	l->end[0] = end[0]; l->end[1] = end[1]; l->end[2] = end[2];
	l->radius = NAV_JUMP_LINK_RADIUS;
	l->bidirectional = 0;
	l->link_type = type;
	l->height_delta = dz;
	l->required_speed = speed;
	l->wait_time = 0;
	fprintf(stderr, "Nav: LINK %s start=(%.0f %.0f %.0f) end=(%.0f %.0f %.0f) dz=%.0f spd=%.0f\n",
		type == 2 ? "JUMP" : type == 3 ? "DROP" : type == 7 ? "RJ" : type == 8 ? "SURF" : "???",
		start[0], start[1], start[2], end[0], end[1], end[2], dz, speed);
	(*n)++;
}

/* Scan outward from edge along normal in cell_size steps to find
   where a floor at target_z begins.  Returns horizontal distance,
   or 0 if the floor is directly below the edge. */
static float nav_find_horizontal_gap(const nav_heightfield_t *hf,
	const float *edge, const float *normal, float target_z, float max_dist)
{
	float step = 4.0f; /* cell_size */
	float probe[3];

	for (float d = step; d <= max_dist; d += step)
	{
		probe[0] = edge[0] + normal[0] * d;
		probe[1] = edge[1] + normal[1] * d;
		probe[2] = edge[2];

		float found_z;
		if (nav_heightfield_floor_z(hf, probe, target_z, &found_z))
		{
			if (fabsf(found_z - target_z) < 8.0f)
				return d;
		}
	}
	return 0;
}


/* Callback for nav_mesh_build: detect drop/jump links from boundary edges.
   Uses Quake physics to compute landing positions and required approach speeds.
   One edge can produce multiple links (one per reachable floor below). */
static int nav_link_callback(
	const nav_mesh_boundary_edge_t *edges, int edge_count,
	const nav_heightfield_t *hf,
	nav_off_mesh_link_t **out_links,
	void *user_data)
{
	(void)user_data;
	int n = 0, cap = 128, i;
	nav_off_mesh_link_t *links = NULL;
	float gravity = sv_gravity.value;
	float maxspeed = sv_maxspeed.value;

	if (gravity < 1.0f) gravity = 800.0f;
	if (maxspeed < 1.0f) maxspeed = 320.0f;
	float peak = NAV_JUMP_IMPULSE * NAV_JUMP_IMPULSE / (2.0f * gravity);

	*out_links = NULL;
	if (edge_count == 0) return 0;

	links = (nav_off_mesh_link_t *)calloc(cap, sizeof(*links));

	for (i = 0; i < edge_count; i++)
	{
		const float *mid = edges[i].midpoint;
		const float *norm = edges[i].normal;
		float short_probe[3];

		/* Short probe outward (8u) to check if there's a wall right at the edge */
		short_probe[0] = mid[0] + norm[0] * 8.0f;
		short_probe[1] = mid[1] + norm[1] * 8.0f;
		short_probe[2] = mid[2];
		if (!nav_trace_clear_at_height(mid, short_probe, mid[2] + 24.0f, NULL))
		{
			continue;
		}

		/* Hull standability: hull-1 extraction emits bevel faces that
		   rasterize as thin phantom shelves (e1m3 z=-134: 4u thick, 70u
		   above the real floor) — mesh and links form on floor that
		   cannot support a player.  Landings are fall-column traced
		   already; starts are not.  Sweep the PLAYER HULL down at the
		   edge: real hull-1 floor catches it (including legit 16u brush
		   overhang), a phantom shelf lets it fall through.  startsolid
		   is inconclusive (cramped rims under stairs, dm4 pocket) —
		   only a CLEAN miss proves there is no floor. */
		{
			vec3_t ds, de, pmins = {-16, -16, -24}, pmaxs = {16, 16, 32};
			trace_t tr;
			ds[0] = mid[0]; ds[1] = mid[1]; ds[2] = mid[2] + 26.0f;
			de[0] = mid[0]; de[1] = mid[1]; de[2] = mid[2];
			tr = SV_Move(ds, pmins, pmaxs, de, MOVE_NOMONSTERS, NULL);
			if (!tr.startsolid && !tr.allsolid && tr.fraction >= 1.0f)
			{
				continue;
			}
		}

		/* ---- Drops: find all floors below ---- */
		{
			float floors[8];
			/* Search as deep as a water plunge allows; each floor past the
			   dry cap is kept only if it's underwater (gated below). */
			float min_z = mid[2] - NAV_WATER_DROP_HEIGHT_MAX;
			float max_z = mid[2] - NAV_DROP_HEIGHT_MIN;
			int nfloors = 0;
			/* Probe outward too: a thin unwalkable ridge at the boundary
			   (hull bevel artifact) hides the landing from the edge column. */
			const float probe_offs[3] = {0.0f, 12.0f, 20.0f};
			for (int pi = 0; pi < 3; pi++)
			{
				float pp[3], pf[8];
				int pn, ti;
				pp[0] = mid[0] + norm[0] * probe_offs[pi];
				pp[1] = mid[1] + norm[1] * probe_offs[pi];
				pp[2] = mid[2];
				pn = nav_heightfield_floors_below(hf, pp, max_z, min_z, pf, 8);
				for (ti = 0; ti < pn && nfloors < 8; ti++)
				{
					int dup = 0;
					for (int di = 0; di < nfloors; di++)
						if (fabsf(floors[di] - pf[ti]) < 2.0f) { dup = 1; break; }
					if (!dup)
						floors[nfloors++] = pf[ti];
				}
			}

			for (int fi = 0; fi < nfloors; fi++)
			{
				float drop_height = mid[2] - floors[fi];
				if (drop_height < NAV_DROP_HEIGHT_MIN)
					continue;

				/* Past the dry-land cap, only a water landing is survivable
				   (and escapable via the surface link); anything else is a
				   killing fall or a dry pit-trap -- leave it unlinked. */
				int deep_water_drop = 0;
				if (drop_height > NAV_DROP_HEIGHT_MAX)
				{
					vec3_t wc;
					wc[0] = mid[0]; wc[1] = mid[1]; wc[2] = floors[fi] + 24.0f;
					if (SV_PointContents(wc) != CONTENTS_WATER)
						continue;
					deep_water_drop = 1;
				}

				/* Verify drop is physically possible: check that there's no solid
				   span blocking the fall at a point outward from the edge.
				   Uses heightfield (no BSP height offset issues). */
				{
					float probe_xy[3];
					probe_xy[0] = mid[0] + norm[0] * 16.0f;
					probe_xy[1] = mid[1] + norm[1] * 16.0f;
					probe_xy[2] = 0;
					if (nav_heightfield_is_blocked(hf, probe_xy, mid[2]))
					{
						continue; /* wall at edge height blocks the drop */
					}
					if (nav_heightfield_is_blocked(hf, probe_xy, floors[fi]))
					{
						continue; /* solid at landing height */
					}
				}

				/* Lane step check: the bot RUNS from the edge to the fall
				   point — anything taller than a step (18u) in the lane
				   stops it on the ground (point traces at +24 sail over
				   20u parapets).  Solid spans reaching above mid+18 at any
				   lane sample mean the run is impossible. */
				{
					int lane_blocked = 0;
					for (float loff = 0.0f; loff <= 16.0f; loff += 4.0f)
					{
						float lp[3];
						lp[0] = mid[0] + norm[0] * loff;
						lp[1] = mid[1] + norm[1] * loff;
						lp[2] = 0;
						if (nav_heightfield_is_blocked(hf, lp, mid[2] + 18.0f))
						{
							lane_blocked = 1;
							break;
						}
					}
					if (lane_blocked)
					{
						continue;
					}
				}

				/* Approach footing: the bot runs up to the edge from
				   behind, so there must be REAL floor there.  The
				   heightfield can't tell — phantom extraction shelves
				   live in it (e1m3 (36,-388,-134): bot walks the shelf
				   toward the rim, falls through into a 64u pit it can't
				   path out of).  Sweep the player hull down at each
				   approach point: real hull-1 floor catches it, a
				   phantom shelf is a clean miss.  startsolid alone is
				   inconclusive (cramped rim, dm4 pocket) and counts as
				   footing; allsolid means the approach corridor is
				   inside hull-1 wall — the bot can never stand there
				   (e1m3 trap rim: allsolid at every offset, yet the
				   heightfield shows a walkable shelf). */
				{
					int footing = 0;
					for (float boff = 12.0f; boff <= 28.0f; boff += 8.0f)
					{
						vec3_t ds, de, pmins = {-16, -16, -24}, pmaxs = {16, 16, 32};
						trace_t tr;
						ds[0] = mid[0] - norm[0] * boff;
						ds[1] = mid[1] - norm[1] * boff;
						ds[2] = mid[2] + 26.0f;
						de[0] = ds[0]; de[1] = ds[1]; de[2] = mid[2];
						tr = SV_Move(ds, pmins, pmaxs, de, MOVE_NOMONSTERS, NULL);
						if (!tr.allsolid && (tr.startsolid || tr.fraction < 1.0f))
						{
							footing = 1;
							break;
						}
					}
					if (!footing)
					{
						continue;
					}
				}

				/* Fall time from physics: t = sqrt(2h / g) */
				float fall_time = sqrtf(2.0f * drop_height / gravity);

				/* Find horizontal gap to the landing surface */
				float gap = nav_find_horizontal_gap(hf, mid, norm, floors[fi], 128.0f);

				/* Required speed to clear the gap */
				float speed;
				if (gap < 4.0f)
					speed = 10.0f; /* step-off: minimal speed */
				else
					speed = gap / fall_time;

				/* Half run speed, not max: the steering corner sits AT the
				   edge, so the bot may arrive slow with no runway — a gap
				   needing a flat-out sprint drops it into the chasm instead
				   of the landing (dm1 (142,1558): 108u gap, 212 u/s). */
				if (speed > maxspeed * 0.5f)
					continue; /* needs more run-up than traversal guarantees */

				/* Landing position: edge + normal * landing_dist */
				float land_dist = speed * fall_time;
				/* Add small margin (8u) past the gap edge */
				if (gap > 4.0f)
					land_dist = gap + 8.0f;

				float end[3];
				end[0] = mid[0] + norm[0] * land_dist;
				end[1] = mid[1] + norm[1] * land_dist;
				end[2] = floors[fi];

				/* Verify landing is clear */
				if (hf && nav_heightfield_is_blocked(hf, end, floors[fi]))
				{
					continue;
				}

				/* Full-length BSP traces.  The 16u heightfield probes above
				   miss thick walls (dm4 x=192 wall: link probed through it)
				   and landings tucked under the start floor. */
				if (!nav_trace_clear_at_height(mid, end, mid[2] + 24.0f, NULL))
				{
					continue;
				}
				{
					vec3_t fs, fe, zero3 = {0, 0, 0};
					trace_t tr;
					fs[0] = end[0]; fs[1] = end[1]; fs[2] = mid[2] + 24.0f;
					fe[0] = end[0]; fe[1] = end[1]; fe[2] = floors[fi] + 4.0f;
					tr = SV_Move(fs, zero3, zero3, fe, MOVE_NOMONSTERS, NULL);
					if (tr.startsolid || tr.allsolid || tr.fraction < 1.0f)
					{
						continue; /* fall column obstructed */
					}
				}

				/* Hull-truth the fall column.  The point/voxel checks above
				   pass through slots the heightfield sees as open but a
				   player can't fit (e1m1 pool ledge: sub-hull gap between
				   walkway and wall spawned a phantom drop that pinned bots
				   on the lip).  Sweep the real player hull down; it can't
				   sit closer than its half-width to the drop face, so clamp
				   the column at least 18u out — step-off landings end up
				   there anyway once the hull clips the wall. */
				{
					vec3_t fs, fe, hmins = {-16, -16, -24}, hmaxs = {16, 16, 32};
					trace_t tr;
					float col_dist = land_dist;
					if (col_dist < 18.0f)
						col_dist = 18.0f;
					fs[0] = mid[0] + norm[0] * col_dist;
					fs[1] = mid[1] + norm[1] * col_dist;
					fs[2] = mid[2] + 26.0f;
					fe[0] = fs[0]; fe[1] = fs[1]; fe[2] = floors[fi] + 24.0f;
					tr = SV_Move(fs, hmins, hmaxs, fe, MOVE_NOMONSTERS, NULL);
					if (tr.startsolid || tr.allsolid
						|| tr.endpos[2] > floors[fi] + 36.0f)
					{
						continue; /* player hull can't ride the column down */
					}
				}

				/* Never link a drop that lands in lava or slime: it's a suicide
				   chute (dm4 green-armor ledge dropped bots into the lava once the
				   192u cap reached it).  Water is survivable, gate only the deadly. */
				{
					vec3_t lc;
					int lcont;
					lc[0] = end[0]; lc[1] = end[1]; lc[2] = floors[fi] + 8.0f;
					lcont = SV_PointContents(lc);
					if (lcont == CONTENTS_LAVA || lcont == CONTENTS_SLIME)
						continue;
				}

				nav_link_push(&links, &n, &cap, mid, end, AI_DROP, speed, -drop_height);

				/* Deep water plunge: pair it with an AI_SURFACE swim-out so the
				   pool isn't a one-way grave.  Only when the ledge sits nearly
				   straight above the landing (the bot swims up and steps off);
				   a far ledge would just nose the lip underwater. */
				if (deep_water_drop && land_dist <= 48.0f)
					nav_link_push(&links, &n, &cap, end, mid, AI_SURFACE, 0.0f, drop_height);

				/* Reverse: if drop height is within jump reach, also create
				   a jump link from the landing floor back up to the edge.
				   The bot jumps from below the ledge up to the top.
				   Micro-drops (< jump min) stay one-way: they exist to
				   ESCAPE artifact pockets, and nothing routes into one.
				   No reverse jump out of liquid: jump impulse doesn't
				   apply while swimming, so bots just nose the lip (dm5
				   pool, 20 stalls/run).  Surface links handle water exit. */
				int land_in_liquid;
				{
					vec3_t lc;
					lc[0] = end[0]; lc[1] = end[1]; lc[2] = floors[fi] + 24.0f;
					land_in_liquid = (SV_PointContents(lc) <= CONTENTS_WATER);
				}
				if (!land_in_liquid &&
					drop_height >= NAV_JUMP_HEIGHT_MIN && drop_height <= peak)
				{
					float disc = NAV_JUMP_IMPULSE * NAV_JUMP_IMPULSE - 2.0f * gravity * drop_height;
					if (disc >= 0)
					{
						float time_up = (NAV_JUMP_IMPULSE - sqrtf(disc)) / gravity;
						float jspeed = land_dist / time_up;
						if (jspeed <= maxspeed)
						{
							if (jspeed < 10.0f) jspeed = 10.0f;
							/* Jump: start at landing, end at edge */
							nav_link_push(&links, &n, &cap, end, mid, AI_JUMP, jspeed, drop_height);
						}
					}
				}
			}
		}

		/* ---- Jumps: scan outward for floors above within jump reach ----
		   Disabled for now: inverted edge normals kept this scan inert
		   since it was written (0 links on every map), so enabling it
		   alongside the normal fix is an untested behavior change —
		   first suite with it live regressed e1m2/e1m3.  Re-enable as
		   its own change with its own suite run. */
#if 0
		{
			float step;

			/* Probe at multiple distances outward (8u to 64u in cell_size steps) */
			for (step = 8.0f; step <= 64.0f; step += 4.0f)
			{
				float probe[3];
				probe[0] = mid[0] + norm[0] * step;
				probe[1] = mid[1] + norm[1] * step;
				probe[2] = mid[2];

				/* Check for wall at jump apex height — the bot jumps OVER the edge */
				if (!nav_trace_clear_at_height(mid, probe, mid[2] + peak + NAV_PLAYER_FLOOR_OFFSET, NULL))
					break; /* wall at jump height — no point probing further */

				/* Find a floor ABOVE the edge at the probe point. */
				float land_z;
				if (!nav_heightfield_floor_above(hf, probe,
						mid[2] + NAV_JUMP_HEIGHT_MIN, mid[2] + peak, &land_z))
					continue;

				float jump_height = land_z - mid[2];
				if (jump_height < NAV_JUMP_HEIGHT_MIN || jump_height > peak)
					continue;

				/* Time to reach jump_height:
				   h = v0*t - 0.5*g*t^2  →  t = (v0 - sqrt(v0^2 - 2*g*h)) / g */
				float disc = NAV_JUMP_IMPULSE * NAV_JUMP_IMPULSE - 2.0f * gravity * jump_height;
				if (disc < 0) continue;
				float time_up = (NAV_JUMP_IMPULSE - sqrtf(disc)) / gravity;
				float speed = step / time_up;

				if (speed > maxspeed)
					continue;
				if (speed < 10.0f) speed = 10.0f;

				float end[3];
				end[0] = probe[0];
				end[1] = probe[1];
				end[2] = land_z;

				nav_link_push(&links, &n, &cap, mid, end, AI_JUMP, speed, jump_height);
				break; /* found a jump at this edge, don't create duplicates */
			}
		}
#endif
	}

	/* Log wall rejection stats for lower corridor */
	{
		int lower_total = 0, lower_wall = 0;
		for (int ei = 0; ei < edge_count; ei++)
		{
			if (edges[ei].midpoint[2] < -100 && edges[ei].midpoint[2] > -160)
			{
				lower_total++;
				float sp[3];
				sp[0] = edges[ei].midpoint[0] + edges[ei].normal[0] * 8.0f;
				sp[1] = edges[ei].midpoint[1] + edges[ei].normal[1] * 8.0f;
				sp[2] = edges[ei].midpoint[2];
				if (!nav_trace_clear_at_height(edges[ei].midpoint, sp, edges[ei].midpoint[2] + 24.0f, NULL))
					lower_wall++;
			}
		}
		fprintf(stderr, "Nav: lower corridor edges: %d total, %d wall-rejected, %d pass\n",
			lower_total, lower_wall, lower_total - lower_wall);
	}

	/* Log edge height distribution */
	{
		int z_hist[20];
		memset(z_hist, 0, sizeof(z_hist));
		for (int ei = 0; ei < edge_count; ei++)
		{
			int bucket = (int)((edges[ei].midpoint[2] + 400) / 50);
			if (bucket >= 0 && bucket < 20) z_hist[bucket]++;
		}
		fprintf(stderr, "Nav: edge Z distribution (%d edges):", edge_count);
		for (int bi = 0; bi < 20; bi++)
			if (z_hist[bi]) fprintf(stderr, " [%.0f]=% d", bi * 50.0f - 400, z_hist[bi]);
		fprintf(stderr, "\n");
	}

	*out_links = links;
	{
		int nj = 0, nd = 0, nr = 0, li;
		for (li = 0; li < n; li++)
		{
			if (links[li].link_type == AI_JUMP) nj++;
			else if (links[li].link_type == AI_DROP) nd++;
			else if (links[li].link_type == AI_SUPER_JUMP) nr++;
		}
		fprintf(stderr, "Nav: detected %d links from %d edges (%d jump, %d drop, %d rj)\n",
			n, edge_count, nj, nd, nr);
	}
	return n;
}

// ===== END verbatim =====

extern "C" int navcheck_link_callback(
	const nav_mesh_boundary_edge_t *edges, int edge_count,
	const nav_heightfield_t *hf, nav_off_mesh_link_t **out_links, void *user_data)
{
	return nav_link_callback(edges, edge_count, hf, out_links, user_data);
}

// ===== validator for orphan/directed link passes (verbatim) =====
static int nav_link_validate(const float *from, const float *to, void *user)
{
	vec3_t pmins = {-16, -16, -24}, pmaxs = {16, 16, 32}, zero = {0, 0, 0};
	vec3_t ts, te;
	trace_t tr;
	float dz, adz, dx, dy, hd, disc, airtime;
	const float g = 800.0f;                 /* sv_gravity */
	const float v0 = NAV_JUMP_IMPULSE;       /* 270, jump up-velocity */
	const float maxspeed = 320.0f;           /* ground run speed */
	(void)user;

	dz = to[2] - from[2];
	dx = to[0] - from[0]; dy = to[1] - from[1];
	hd = sqrt(dx * dx + dy * dy);
	if (hd < 8.0f)
		return 0;
	adz = dz < 0 ? -dz : dz;

	/* WALK: a player box sweeps level from 'from' to 'to' (lifted one step) and
	   reaches it -> the floor is continuous and only the mesh adjacency is
	   missing.  Step height bounds the climb. */
	if (adz <= NAV_JUMP_HEIGHT_MIN)
	{
		ts[0] = from[0]; ts[1] = from[1]; ts[2] = from[2] + 24 + 18;
		te[0] = to[0]; te[1] = to[1]; te[2] = to[2] + 24 + 18;
		tr = SV_Move(ts, pmins, pmaxs, te, MOVE_NOMONSTERS, NULL);
		if (!tr.startsolid && tr.fraction > 0.97f)
			return AI_WALK;
	}

	/* JUMP: bidirectional, so the harder (upward) direction governs -- clear
	   |dz| against gravity (apex v0^2/2g ~45u), run distance <= maxspeed *
	   air time, and the apex arc must be wall-free. */
	disc = v0 * v0 - 2.0f * g * adz;
	if (disc < 0.0f)
	{
		/* Over a normal jump's reach.  A rocket jump can still get UP to a
		   ledge (orphan higher than here) within the RJ envelope: the manual
		   graphs tag exactly these edges AI_SUPER_JUMP.  Near-vertical only --
		   keep horizontal tight so RJ never replaces a run-jump across.  The
		   caller pairs this with a drop-out so the ledge isn't a one-way trap. */
		if (dz > NAV_JUMP_HEIGHT_MAX && dz <= NAV_RJ_HEIGHT_MAX && hd <= NAV_RJ_HORIZ_MAX)
		{
			float topz = to[2] + 24.0f + 18.0f;
			ts[0] = from[0]; ts[1] = from[1]; ts[2] = topz;
			te[0] = to[0]; te[1] = to[1]; te[2] = topz;
			tr = SV_Move(ts, zero, zero, te, MOVE_NOMONSTERS, NULL);
			if (tr.fraction >= 1.0f)
				return AI_SUPER_JUMP;
		}
		return 0;
	}
	airtime = (v0 + sqrt(disc)) / g;
	if (hd / airtime > maxspeed)
		return 0;
	{
		float apexz = (to[2] > from[2] ? to[2] : from[2]) + 45.0f + 24.0f;
		ts[0] = from[0]; ts[1] = from[1]; ts[2] = apexz;
		te[0] = to[0]; te[1] = to[1]; te[2] = apexz;
		tr = SV_Move(ts, zero, zero, te, MOVE_NOMONSTERS, NULL);
		if (tr.fraction < 1.0f)
			return 0;
	}
	return AI_JUMP;
}

extern "C" int navcheck_link_validate(const float *from, const float *to, void *user)
{
	return nav_link_validate(from, to, user);
}
