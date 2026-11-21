package config

import (
	"bytes"
	"encoding/json"
	"io"
	"strings"

	"github.com/ShitBot-Team/ShitBotUltra/internal/fault"
	"gopkg.in/yaml.v3"
)

// SectionText changes only selected top-level values. All sections are validated
// together, allowing clients to rename a node and its references atomically.
// Anchors/aliases/merge keys require the source editor: replacing an anchored
// section could silently change another section or leave a dangling alias.
func SectionText(text string, sections map[string]json.RawMessage) (string, error) {
	if len(sections) == 0 {
		return "", fault.New("INVALID_INPUT", "请提供至少一个配置分区")
	}
	cfg, err := Decode([]byte(text))
	if err != nil {
		return "", fault.New("INVALID_CONFIG", err.Error())
	}
	var document yaml.Node
	if err := yaml.Unmarshal([]byte(text), &document); err != nil {
		return "", fault.New("INVALID_CONFIG", err.Error())
	}
	if len(document.Content) != 1 || document.Content[0].Kind != yaml.MappingNode || linkedYAML(&document) {
		return "", fault.New("INVALID_CONFIG", "表单编辑需要不含锚点、别名或合并键的 YAML 映射；请使用配置原文编辑")
	}
	// Keep ordering deterministic, independent of JSON/map iteration order.
	values := []struct {
		name  string
		value any
	}{
		{"connections", new([]Connection)}, {"nodes", new([]Node)},
		{"routes", new([]Route)}, {"commands", new([]Command)},
		{"tasks", new([]Task)},
		{"binding", &cfg.Binding}, {"rendering", &cfg.Rendering},
	}
	known := make(map[string]bool, len(values))
	for _, section := range values {
		known[section.name] = true
	}
	for name := range sections {
		if !known[name] {
			return "", fault.New("INVALID_INPUT", "不支持的配置分区："+name)
		}
	}
	root := document.Content[0]
	for _, section := range values {
		raw, exists := sections[section.name]
		if !exists {
			continue
		}
		if strings.TrimSpace(string(raw)) == "null" {
			return "", fault.New("INVALID_INPUT", "配置分区不能为 null")
		}
		decoder := json.NewDecoder(bytes.NewReader(raw))
		decoder.DisallowUnknownFields()
		if err := decoder.Decode(section.value); err != nil {
			return "", fault.New("INVALID_INPUT", section.name+" 分区格式无效："+err.Error())
		}
		if err := decoder.Decode(new(any)); err != io.EOF {
			return "", fault.New("INVALID_INPUT", "配置分区必须只有一个 JSON 值")
		}
		var replacement yaml.Node
		if err := replacement.Encode(section.value); err != nil {
			return "", err
		}
		found := false
		for i := 0; i < len(root.Content); i += 2 {
			if root.Content[i].Value == section.name {
				old := root.Content[i+1]
				replacement.HeadComment, replacement.LineComment, replacement.FootComment = old.HeadComment, old.LineComment, old.FootComment
				root.Content[i+1] = &replacement
				found = true
				break
			}
		}
		if !found {
			root.Content = append(root.Content, &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: section.name}, &replacement)
		}
	}
	var result bytes.Buffer
	encoder := yaml.NewEncoder(&result)
	encoder.SetIndent(2)
	if err := encoder.Encode(&document); err != nil {
		return "", err
	}
	if err := encoder.Close(); err != nil {
		return "", err
	}
	// Validate the actual serialized document, including cross-section references.
	if _, err := Decode(result.Bytes()); err != nil {
		return "", fault.New("INVALID_CONFIG", err.Error())
	}
	return result.String(), nil
}

func linkedYAML(node *yaml.Node) bool {
	if node.Anchor != "" || node.Kind == yaml.AliasNode || node.Tag == "!!merge" {
		return true
	}
	for _, child := range node.Content {
		if linkedYAML(child) {
			return true
		}
	}
	return false
}
