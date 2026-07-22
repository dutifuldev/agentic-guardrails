package rules

import (
	"strings"

	"github.com/osolmaz/slophammer/go/internal/repo"
	"gopkg.in/yaml.v3"
)

func hasGolangCIConfig(snapshot repo.Snapshot) bool {
	return len(golangCIConfigFiles(snapshot)) > 0
}

func hasGolangCICommand(snapshot repo.Snapshot) bool {
	if hasCommand(snapshot, "golangci/golangci-lint-action") {
		return true
	}
	return hasRunnableCommandLine(snapshot, lineHasGolangCICommand)
}

func golangCIConfigFiles(snapshot repo.Snapshot) []repo.File {
	return snapshot.FilesNamedFold(".golangci.yml", ".golangci.yaml")
}

func hasCoverageThreshold(content string) bool {
	return coverageThresholdPattern.MatchString(content) || strictCoverageThresholdPattern.MatchString(content)
}

func hasCoverageGateThreshold(content string) bool {
	if !hasCoverageThreshold(content) {
		return false
	}
	lower := strings.ToLower(content)
	return strings.Contains(lower, "cover") ||
		strings.Contains(lower, "coverage") ||
		strings.Contains(lower, "total")
}

func hasCRAPThreshold(content string) bool {
	return crapThresholdPattern.MatchString(content) || strictCRAPThresholdPattern.MatchString(content)
}

func configEnablesComplexityLinter(content string) bool {
	var document yaml.Node
	if err := yaml.Unmarshal([]byte(content), &document); err != nil {
		return false
	}
	root := yamlRoot(&document)
	linters := yamlMappingValue(root, "linters")
	disable := yamlMappingValue(linters, "disable")
	if yamlScalarEquals(yamlMappingValue(linters, "default"), "all") {
		return !yamlSequenceContainsAll(disable, "cyclop", "gocognit", "gocyclo")
	}
	enable := yamlMappingValue(linters, "enable")
	return yamlSequenceContainsEnabled(enable, disable, "cyclop", "gocognit", "gocyclo")
}

func yamlRoot(node *yaml.Node) *yaml.Node {
	if node != nil && node.Kind == yaml.DocumentNode && len(node.Content) > 0 {
		return node.Content[0]
	}
	return node
}

func yamlMappingValue(node *yaml.Node, key string) *yaml.Node {
	if node == nil || node.Kind != yaml.MappingNode {
		return nil
	}
	for i := 0; i+1 < len(node.Content); i += 2 {
		if node.Content[i].Value == key {
			return node.Content[i+1]
		}
	}
	return nil
}

func yamlSequenceContains(node *yaml.Node, values ...string) bool {
	if node == nil || node.Kind != yaml.SequenceNode {
		return false
	}
	for _, item := range node.Content {
		for _, value := range values {
			if item.Value == value {
				return true
			}
		}
	}
	return false
}

func yamlSequenceContainsAll(node *yaml.Node, values ...string) bool {
	for _, value := range values {
		if !yamlSequenceContains(node, value) {
			return false
		}
	}
	return true
}

func yamlSequenceContainsEnabled(enable *yaml.Node, disable *yaml.Node, values ...string) bool {
	for _, value := range values {
		if yamlSequenceContains(enable, value) && !yamlSequenceContains(disable, value) {
			return true
		}
	}
	return false
}

func yamlScalarEquals(node *yaml.Node, value string) bool {
	return node != nil && node.Kind == yaml.ScalarNode && node.Value == value
}

// mutate4go-manifest-begin
// {"version":1,"tested_at":"2026-06-12T22:50:21+08:00","module_hash":"85ee3a00af44da68e2e805579b71a5eb2643b444dde165382c1c874ff409f385","functions":[{"id":"func/hasGolangCIConfig","name":"hasGolangCIConfig","line":10,"end_line":12,"hash":"4b60cb37c6ae2f3507fb3e304ac53bbbf6b87c3f08a67758f58bb8bac9b58d02"},{"id":"func/hasGolangCICommand","name":"hasGolangCICommand","line":14,"end_line":19,"hash":"acbcb49e27a02145f091e86bfc3dc76066bf77208822221c24ddbfb507465540"},{"id":"func/golangCIConfigFiles","name":"golangCIConfigFiles","line":21,"end_line":23,"hash":"26e3dfd644f76874b47c9094f21452835a795c1f7b375980182b732d8245b57f"},{"id":"func/hasCoverageThreshold","name":"hasCoverageThreshold","line":25,"end_line":27,"hash":"c1b2f0382ff09fce59b5cc04bf4f909b1b23f7eed006b9f080490f1f7af70858"},{"id":"func/hasCoverageGateThreshold","name":"hasCoverageGateThreshold","line":29,"end_line":37,"hash":"95d8ace9ae090b769b41cadb7914db4a0609c28a00ab6ba6cc0b384cf30e63b0"},{"id":"func/hasCRAPThreshold","name":"hasCRAPThreshold","line":39,"end_line":41,"hash":"f25011ff7a0b8ec3d9a6fb0472299603fa792043aeba6c431fd5080933369820"},{"id":"func/configEnablesComplexityLinter","name":"configEnablesComplexityLinter","line":43,"end_line":56,"hash":"5d1463f51bd12b69722ad6934e9fa937b1d421165553087ef5e9879cac5c0bc7"},{"id":"func/yamlRoot","name":"yamlRoot","line":58,"end_line":63,"hash":"782396992e04df6e2a95d872fae24c39ba7c472adcbfae4f5269c7f409e80dc7"},{"id":"func/yamlMappingValue","name":"yamlMappingValue","line":65,"end_line":75,"hash":"e77dbe077ac8fcd995ce00cd76a3419854ba9ee5cd477fdddc5202eb45504c97"},{"id":"func/yamlSequenceContains","name":"yamlSequenceContains","line":77,"end_line":89,"hash":"de54560e8db408d0d956f04b060883c10a048f0e14e70cd5f22b128d87a7ebb6"},{"id":"func/yamlSequenceContainsAll","name":"yamlSequenceContainsAll","line":91,"end_line":98,"hash":"0109e2dcf2fc9825dd5737606e5c7fe002ebae4864063aeeabbe305be744e7ba"},{"id":"func/yamlSequenceContainsEnabled","name":"yamlSequenceContainsEnabled","line":100,"end_line":107,"hash":"fc0a649e395b6eed52e80b98374abacd3732884385a487ca7bbd2d8cd07909e7"},{"id":"func/yamlScalarEquals","name":"yamlScalarEquals","line":109,"end_line":111,"hash":"c76a766141c523109a19d79bda231e3f1f4d1419133520855560b49df4f4f80a"}]}
// mutate4go-manifest-end
