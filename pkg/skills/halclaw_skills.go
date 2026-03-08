package skills

import "os/exec"

// SkillRequires defines what a skill needs to run.
type SkillRequires struct {
	Bins    []string `json:"bins,omitempty"`
	AnyBins []string `json:"anyBins,omitempty"`
	Config  []string `json:"config,omitempty"`
}

// InstallAction describes how to install a skill dependency.
type InstallAction struct {
	ID      string   `json:"id"`
	Kind    string   `json:"kind"` // "brew", "apt", "npm", "pip", "go"
	Formula string   `json:"formula,omitempty"`
	Package string   `json:"package,omitempty"`
	Module  string   `json:"module,omitempty"`
	Bins    []string `json:"bins,omitempty"`
	Label   string   `json:"label"`
}

// DependencyStatus is the result of checking a single dependency.
type DependencyStatus struct {
	Type      string `json:"type"`      // "bin", "anyBin", "config"
	Name      string `json:"name"`
	Installed bool   `json:"installed"`
}

// SkillDependencyResult is the result of checking all dependencies for a skill.
type SkillDependencyResult struct {
	AllSatisfied bool               `json:"allSatisfied"`
	Dependencies []DependencyStatus `json:"dependencies"`
}

// CheckDependencies verifies that all required dependencies for a skill are met.
// configChecker tests whether a config key exists and is non-empty.
func CheckDependencies(requires *SkillRequires, configChecker func(string) bool) SkillDependencyResult {
	result := SkillDependencyResult{
		AllSatisfied: true,
		Dependencies: []DependencyStatus{},
	}

	if requires == nil {
		return result
	}

	// Check required binaries — all must be present.
	for _, bin := range requires.Bins {
		installed := isBinAvailable(bin)
		result.Dependencies = append(result.Dependencies, DependencyStatus{
			Type:      "bin",
			Name:      bin,
			Installed: installed,
		})
		if !installed {
			result.AllSatisfied = false
		}
	}

	// Check anyBins — at least one must be present.
	if len(requires.AnyBins) > 0 {
		anyFound := false
		for _, bin := range requires.AnyBins {
			installed := isBinAvailable(bin)
			result.Dependencies = append(result.Dependencies, DependencyStatus{
				Type:      "anyBin",
				Name:      bin,
				Installed: installed,
			})
			if installed {
				anyFound = true
			}
		}
		if !anyFound {
			result.AllSatisfied = false
		}
	}

	// Check config keys — all must be set.
	if configChecker != nil {
		for _, key := range requires.Config {
			installed := configChecker(key)
			result.Dependencies = append(result.Dependencies, DependencyStatus{
				Type:      "config",
				Name:      key,
				Installed: installed,
			})
			if !installed {
				result.AllSatisfied = false
			}
		}
	}

	return result
}

// isBinAvailable checks if a binary is available in PATH.
func isBinAvailable(name string) bool {
	_, err := exec.LookPath(name)
	return err == nil
}
