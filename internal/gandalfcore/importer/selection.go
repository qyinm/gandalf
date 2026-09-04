package importer

import (
	"fmt"
	"regexp"
	"sort"

	"github.com/qyinm/gandalf/internal/gandalfcore/manifest"
)

// envRefPattern matches ${VAR} references left behind by secret templatization.
var envRefPattern = regexp.MustCompile(`\$\{([A-Za-z_][A-Za-z0-9_]*)\}`)

// FilterManifest returns a copy of m containing only the servers and skills
// included by sel. The env_template is recomputed so it only declares variables
// still referenced by the surviving servers. A nil sel returns m unchanged.
func FilterManifest(m *manifest.Manifest, sel *Selection) *manifest.Manifest {
	if m == nil || sel == nil {
		return m
	}

	filtered := *m

	if len(m.MCPServers) > 0 {
		servers := make(map[string]manifest.MCPServerDef, len(m.MCPServers))
		for name, srv := range m.MCPServers {
			if sel.IncludesServer(name) {
				servers[name] = srv
			}
		}
		filtered.MCPServers = servers
	}

	if len(m.Skills) > 0 {
		skills := make([]manifest.SkillDef, 0, len(m.Skills))
		for _, sk := range m.Skills {
			if sel.IncludesSkill(sk.Name) {
				skills = append(skills, sk)
			}
		}
		filtered.Skills = skills
	}

	filtered.EnvTemplate = filterEnvTemplate(m.EnvTemplate, referencedEnvVars(filtered.MCPServers))
	return &filtered
}

// referencedEnvVars collects the ${VAR} names referenced anywhere in the given
// server definitions (commands, args, env, headers, urls, env files, auth).
func referencedEnvVars(servers map[string]manifest.MCPServerDef) map[string]bool {
	refs := make(map[string]bool)
	collect := func(s string) {
		for _, match := range envRefPattern.FindAllStringSubmatch(s, -1) {
			refs[match[1]] = true
		}
	}
	names := make([]string, 0, len(servers))
	for name := range servers {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		srv := servers[name]
		collect(srv.Command)
		collect(srv.URL)
		collect(srv.EnvFile)
		for _, arg := range srv.Args {
			collect(arg)
		}
		for _, v := range srv.Env {
			collect(v)
		}
		for _, v := range srv.Headers {
			collect(v)
		}
		switch auth := srv.Auth.(type) {
		case string:
			collect(auth)
		case map[string]any:
			for _, v := range auth {
				collect(fmt.Sprintf("%v", v))
			}
		}
		// required_env declares variables by name (not by ${} reference);
		// keep their template entries when the server survives filtering.
		for _, req := range srv.RequiredEnv {
			refs[req] = true
		}
	}
	return refs
}

// filterEnvTemplate keeps only template entries still referenced by the
// surviving servers. If nothing is referenced anymore the result is empty.
func filterEnvTemplate(template map[string]string, refs map[string]bool) map[string]string {
	if len(template) == 0 {
		return template
	}
	filtered := make(map[string]string, len(template))
	for k, v := range template {
		if refs[k] {
			filtered[k] = v
		}
	}
	return filtered
}
