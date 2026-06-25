package tools

import "testing"

func Test_Registry_Definitions_include_parameters_object_schema(t *testing.T) {
	// Given
	registry := NewRegistry(t.TempDir())

	// When
	definitions := registry.Definitions()

	// Then
	if len(definitions) == 0 {
		t.Fatal("expected built-in tool definitions")
	}
	for _, definition := range definitions {
		function, ok := definition["function"].(map[string]any)
		if !ok {
			t.Fatalf("definition missing function object: %#v", definition)
		}
		name, _ := function["name"].(string)
		parameters, ok := function["parameters"].(map[string]any)
		if !ok {
			t.Fatalf("tool %q missing function.parameters object schema", name)
		}
		if got, ok := parameters["type"].(string); !ok || got != "object" {
			t.Fatalf("tool %q function.parameters.type = %#v, want object", name, parameters["type"])
		}
		if _, ok := parameters["properties"].(map[string]any); !ok {
			t.Fatalf("tool %q function.parameters.properties missing object", name)
		}
	}
}
