// qents — edict/sv shim backed by the BSP entity lump. See qents.h.

#include "qents.h"

#include <cstdio>
#include <cstdlib>
#include <cstring>
#include <string>
#include <vector>
#include <unordered_map>

qsv_t sv = {0, nullptr, nullptr, 0};
char *pr_strings = nullptr;
qcvar_t nav_debug_cvar = {0.0f};

namespace {
std::string g_strpool;
std::vector<model_t *> g_modelptrs;

int intern(const std::string &s) {
	int off = (int)g_strpool.size();
	g_strpool += s;
	g_strpool += '\0';
	return off;
}

// Parse { "k" "v" ... } blocks from the entity lump.
std::vector<std::unordered_map<std::string,std::string>> parse(const char *s) {
	std::vector<std::unordered_map<std::string,std::string>> out;
	const char *p = s;
	while (*p) {
		if (*p != '{') { p++; continue; }
		p++;
		std::unordered_map<std::string,std::string> kv;
		while (*p && *p != '}') {
			while (*p && *p != '"' && *p != '}') p++;
			if (*p != '"') break;
			p++; std::string k; while (*p && *p != '"') k += *p++; if (*p == '"') p++;
			while (*p && *p != '"' && *p != '}') p++;
			if (*p != '"') break;
			p++; std::string v; while (*p && *p != '"') v += *p++; if (*p == '"') p++;
			kv[k] = v;
		}
		if (*p == '}') p++;
		out.push_back(std::move(kv));
	}
	return out;
}

void vec3(const std::string &s, vec3_t out) {
	out[0] = out[1] = out[2] = 0;
	if (!s.empty()) sscanf(s.c_str(), "%f %f %f", &out[0], &out[1], &out[2]);
}

// QuakeC spawn renames brush entities; the nav code matches the renamed forms.
std::string rename_class(const std::string &c) {
	if (c == "func_door" || c == "func_door_secret") return "door";
	if (c == "func_plat") return "plat";
	if (c == "func_train") return "train";
	return c;
}
} // namespace

void qents_load(void)
{
	auto ents = parse(qworld_entities());
	g_strpool.clear();
	intern(""); // offset 0 = empty string

	int n = (int)ents.size();
	sv.edicts = (edict_t *)calloc(n + 1, sizeof(edict_t)); // [0] = world
	sv.num_edicts = n + 1;

	// model pointer table indexed by submodel number.
	g_modelptrs.assign(qworld_num_models(), nullptr);
	for (int i = 0; i < qworld_num_models(); i++) g_modelptrs[i] = qworld_model(i);
	sv.models = g_modelptrs.data();
	sv.nummodels = (int)g_modelptrs.size();

	for (int i = 0; i < n; i++) {
		auto &kv = ents[i];
		edict_t *e = &sv.edicts[i + 1];
		e->free = 0;
		auto get = [&](const char *k) -> std::string {
			auto it = kv.find(k); return it == kv.end() ? std::string() : it->second;
		};
		std::string cls = rename_class(get("classname"));
		e->v.classname   = intern(cls);
		e->v.target      = intern(get("target"));
		e->v.targetname  = intern(get("targetname"));
		vec3(get("origin"), e->v.origin);

		// Brush entity: model "*N" -> submodel bbox.
		std::string mdl = get("model");
		if (!mdl.empty() && mdl[0] == '*') {
			int idx = atoi(mdl.c_str() + 1);
			e->v.modelindex = (float)idx;
			model_t *m = qworld_model(idx);
			if (m) {
				for (int k = 0; k < 3; k++) {
					e->v.mins[k]   = m->mins[k];
					e->v.maxs[k]   = m->maxs[k];
					e->v.size[k]   = m->maxs[k] - m->mins[k];
					e->v.absmin[k] = e->v.origin[k] + m->mins[k];
					e->v.absmax[k] = e->v.origin[k] + m->maxs[k];
				}
			}
		}

		// func_plat pos1/pos2 (plats.qc): pos1=origin (top), pos2 below by
		// height (or size_z - 8). speed defaults handled by the collector.
		if (cls == "plat") {
			e->has_pos = 1;
			VectorCopy(e->v.origin, e->pos1.vector);
			VectorCopy(e->v.origin, e->pos2.vector);
			float height = 0; std::string hs = get("height");
			if (!hs.empty()) height = (float)atof(hs.c_str());
			e->pos2.vector[2] = e->v.origin[2] - (height > 0 ? height : e->v.size[2] - 8.0f);
			std::string sp = get("speed");
			e->speed._float = sp.empty() ? 0.0f : (float)atof(sp.c_str());
		}
	}

	pr_strings = (char *)malloc(g_strpool.size());
	memcpy(pr_strings, g_strpool.data(), g_strpool.size());
}

void qents_free(void)
{
	free(sv.edicts); free(pr_strings);
	sv.edicts = nullptr; pr_strings = nullptr; sv.num_edicts = 0;
}

eval_t *GetEdictFieldValue(edict_t *e, const char *field)
{
	if (!e->has_pos) return nullptr;
	if (!strcmp(field, "pos1")) return &e->pos1;
	if (!strcmp(field, "pos2")) return &e->pos2;
	if (!strcmp(field, "speed")) return &e->speed;
	return nullptr;
}
