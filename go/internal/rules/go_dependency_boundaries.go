package rules

import (
	"context"
	"fmt"
	"go/parser"
	"go/token"
	"path"
	"sort"
	"strconv"
	"strings"

	"github.com/osolmaz/slophammer/go/internal/config"
	"github.com/osolmaz/slophammer/go/internal/repo"
)

type goDependencyBoundariesRule struct {
	definition Definition
}

func newGoDependencyBoundariesRule(definition Definition) Rule {
	return goDependencyBoundariesRule{definition: definition}
}

func (r goDependencyBoundariesRule) Metadata() Metadata {
	return r.definition.Metadata()
}

func (r goDependencyBoundariesRule) Check(context.Context, repo.Snapshot) []Finding {
	return nil
}

func (r goDependencyBoundariesRule) CheckWithConfig(_ context.Context, snapshot repo.Snapshot, cfg config.Config) []Finding {
	if len(cfg.Go.DependencyBoundaries) == 0 {
		return nil
	}
	moduleRoots := goModuleRoots(snapshot)
	modulePaths := goModulePaths(snapshot, moduleRoots)
	findingsByKey := map[string]Finding{}
	for _, file := range goSourceFiles(snapshot) {
		root := sourceModuleRoot(file.Path, moduleRoots)
		for _, importPath := range goImports(file) {
			localPath, ok := localImportPath(importPath, root, modulePaths[root])
			if !ok {
				continue
			}
			for _, boundary := range cfg.Go.DependencyBoundaries {
				if boundaryAllowsImport(file.Path, localPath, root, boundary) {
					continue
				}
				key := file.Path + "\x00" + localPath
				findingsByKey[key] = Finding{
					RuleID:   r.definition.ID,
					Severity: r.definition.Severity,
					Path:     file.Path,
					Message:  fmt.Sprintf("%s imports %s outside configured dependency boundaries", file.Path, localPath),
				}
			}
		}
	}
	return sortedFindings(findingsByKey)
}

func goModuleRoots(snapshot repo.Snapshot) []string {
	roots := goProjectRoots(snapshot)
	if len(roots) == 0 {
		return []string{""}
	}
	return roots
}

func goModulePaths(snapshot repo.Snapshot, roots []string) map[string]string {
	paths := map[string]string{}
	for _, root := range roots {
		filePath := path.Join(root, "go.mod")
		if root == "" {
			filePath = "go.mod"
		}
		file, ok := snapshot.Files[filePath]
		if !ok {
			continue
		}
		if modulePath := parseModulePath(file.Content); modulePath != "" {
			paths[root] = modulePath
		}
	}
	return paths
}

func parseModulePath(content string) string {
	for _, line := range strings.Split(content, "\n") {
		fields := strings.Fields(line)
		if len(fields) >= 2 && fields[0] == "module" {
			return strings.Trim(fields[1], `"`)
		}
	}
	return ""
}

func goSourceFiles(snapshot repo.Snapshot) []repo.File {
	files := make([]repo.File, 0)
	for filePath, file := range snapshot.Files {
		if isEmbeddedFixturePath(filePath) || !strings.HasSuffix(filePath, ".go") {
			continue
		}
		files = append(files, file)
	}
	sort.Slice(files, func(i, j int) bool {
		return files[i].Path < files[j].Path
	})
	return files
}

func sourceModuleRoot(filePath string, roots []string) string {
	for _, root := range roots {
		if root != "" && strings.HasPrefix(filePath, root+"/") {
			return root
		}
	}
	return ""
}

func goImports(file repo.File) []string {
	parsed, err := parser.ParseFile(token.NewFileSet(), file.Path, file.Content, parser.ImportsOnly)
	if err != nil {
		return nil
	}
	imports := make([]string, 0, len(parsed.Imports))
	for _, spec := range parsed.Imports {
		importPath, err := strconv.Unquote(spec.Path.Value)
		if err == nil {
			imports = append(imports, importPath)
		}
	}
	return imports
}

func localImportPath(importPath string, root string, modulePath string) (string, bool) {
	if modulePath == "" {
		return "", false
	}
	if importPath == modulePath {
		return cleanBoundaryPath(root), true
	}
	prefix := modulePath + "/"
	if !strings.HasPrefix(importPath, prefix) {
		return "", false
	}
	return cleanBoundaryPath(path.Join(root, strings.TrimPrefix(importPath, prefix))), true
}

func boundaryAllowsImport(filePath string, localImport string, moduleRoot string, boundary config.DependencyBoundary) bool {
	if !pathMatchesBoundary(filePath, moduleRoot, boundary.From) {
		return true
	}
	for _, allow := range boundary.Allow {
		if pathMatchesBoundary(localImport, moduleRoot, allow) {
			return true
		}
	}
	return false
}

func pathMatchesBoundary(filePath string, moduleRoot string, boundaryPath string) bool {
	candidate := cleanBoundaryPath(boundaryPath)
	if pathHasPrefix(filePath, candidate) {
		return true
	}
	if moduleRoot == "" {
		return false
	}
	return pathHasPrefix(filePath, cleanBoundaryPath(path.Join(moduleRoot, candidate)))
}

func pathHasPrefix(filePath string, prefix string) bool {
	return filePath == prefix || strings.HasPrefix(filePath, prefix+"/")
}

func cleanBoundaryPath(value string) string {
	return strings.TrimPrefix(path.Clean(strings.ReplaceAll(value, "\\", "/")), "./")
}

func sortedFindings(findingsByKey map[string]Finding) []Finding {
	keys := make([]string, 0, len(findingsByKey))
	for key := range findingsByKey {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	findings := make([]Finding, 0, len(keys))
	for _, key := range keys {
		findings = append(findings, findingsByKey[key])
	}
	return findings
}

// mutate4go-manifest-begin
// {"version":1,"tested_at":"2026-06-12T22:50:22+08:00","module_hash":"dada5749f24402eca2da2f7ac1fb5e3aaa295760e33b45195e7a3054e64788c0","functions":[{"id":"func/newGoDependencyBoundariesRule","name":"newGoDependencyBoundariesRule","line":21,"end_line":23,"hash":"91d34cdd1ca0a2582a6f2098df8f797f03fa25053e49fabe10422bcdfebcc28f"},{"id":"func/goDependencyBoundariesRule.Metadata","name":"goDependencyBoundariesRule.Metadata","line":25,"end_line":27,"hash":"f4deddbd4ce8cb3615b346c4df31b0c2f3bbaf531e0fbb493142d1b6147f01c4"},{"id":"func/goDependencyBoundariesRule.Check","name":"goDependencyBoundariesRule.Check","line":29,"end_line":31,"hash":"6fd96913b35672d2e2b71281a270cb05db03f89692d588745504b44c80ed66a9"},{"id":"func/goDependencyBoundariesRule.CheckWithConfig","name":"goDependencyBoundariesRule.CheckWithConfig","line":33,"end_line":62,"hash":"432976971a59ba8474049875a0f9aee731a65ce6bebd919c6c3420f122518f27"},{"id":"func/goModuleRoots","name":"goModuleRoots","line":64,"end_line":70,"hash":"8267212533df984ce4695131ab0a5a101bb1efabe9a78883189778172989075d"},{"id":"func/goModulePaths","name":"goModulePaths","line":72,"end_line":88,"hash":"c2fb4fb6c159c66d2a1d2cc6eeee2f141aca3e6c7a06a952699569227324f4bf"},{"id":"func/parseModulePath","name":"parseModulePath","line":90,"end_line":98,"hash":"7471b8668f0bbfdbf015ce38278554a55fa4d82cca63eff8016b719137031125"},{"id":"func/goSourceFiles","name":"goSourceFiles","line":100,"end_line":112,"hash":"67881abf79e72528226c627039db91520bfcd8d9406943a7f2449b3b39f5c87e"},{"id":"func/sourceModuleRoot","name":"sourceModuleRoot","line":114,"end_line":121,"hash":"73dd233cb686f5eca388fef6179485c7a1b1db0577ee1cfdf2a43bc755fd575b"},{"id":"func/goImports","name":"goImports","line":123,"end_line":136,"hash":"16c1975ef2cbbc156285378c59603201618e704700f7427cfae4fa7c948dc12a"},{"id":"func/localImportPath","name":"localImportPath","line":138,"end_line":150,"hash":"2fd6fafd18df364ad5d10a3123d19ed83aed89081e65e75ecdd22ac56d75618b"},{"id":"func/boundaryAllowsImport","name":"boundaryAllowsImport","line":152,"end_line":162,"hash":"19209d7d0c4c83d2de95d01161024e63a365e193f2f717b230ed71d97c04638d"},{"id":"func/pathMatchesBoundary","name":"pathMatchesBoundary","line":164,"end_line":173,"hash":"7f0b9f07b9c2e2d1a9de08058de7651de1ec8bb864a2b5b929178647622bf531"},{"id":"func/pathHasPrefix","name":"pathHasPrefix","line":175,"end_line":177,"hash":"6d0e99ccdb8c21f076a1809ba6d9e7826df8f37defd11ae8d42f211985989529"},{"id":"func/cleanBoundaryPath","name":"cleanBoundaryPath","line":179,"end_line":181,"hash":"3cc5ed31702d4b2167a1ff3c44a5e4da9a1e67fb790ecf8d4c54b5d16bdc7f61"},{"id":"func/sortedFindings","name":"sortedFindings","line":183,"end_line":194,"hash":"e364967585410329024c8f3c86be1472c96c209bdaa23051a326bcf94498604a"}]}
// mutate4go-manifest-end
