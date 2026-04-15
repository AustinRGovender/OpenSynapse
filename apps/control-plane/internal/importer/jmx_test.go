package importer

import (
	"os"
	"strings"
	"testing"
)

func TestImportJMXFixture(t *testing.T) {
	f, err := os.Open("../../fixtures/sample.jmx")
	if err != nil {
		t.Fatalf("open fixture: %v", err)
	}
	defer f.Close()

	result, err := ImportJMX(f)
	if err != nil {
		t.Fatalf("import: %v", err)
	}

	if result.Plan == nil {
		t.Fatal("expected non-nil plan")
	}

	if result.Plan.Name == "" {
		t.Fatal("expected non-empty plan name")
	}

	// Should have imported elements
	if len(result.Log.Imported) == 0 {
		t.Fatal("expected at least some imported elements")
	}

	// Check that key elements were imported
	importedStr := strings.Join(result.Log.Imported, " | ")

	if !strings.Contains(importedStr, "ThreadGroup") {
		t.Fatal("expected ThreadGroup in imported log")
	}
	if !strings.Contains(importedStr, "HTTPSamplerProxy") {
		t.Fatal("expected HTTPSamplerProxy in imported log")
	}
	if !strings.Contains(importedStr, "OnceOnlyController") {
		t.Fatal("expected OnceOnlyController in imported log")
	}
	if !strings.Contains(importedStr, "TransactionController") {
		t.Fatal("expected TransactionController in imported log")
	}
	if !strings.Contains(importedStr, "LoopController") {
		t.Fatal("expected LoopController in imported log")
	}
	if !strings.Contains(importedStr, "ConstantTimer") {
		t.Fatal("expected ConstantTimer in imported log")
	}
	if !strings.Contains(importedStr, "ResponseAssertion") {
		t.Fatal("expected ResponseAssertion in imported log")
	}
	if !strings.Contains(importedStr, "CSVDataSet") {
		t.Fatal("expected CSVDataSet in imported log")
	}
}

func TestImportJMXMinimal(t *testing.T) {
	jmx := `<?xml version="1.0" encoding="UTF-8"?>
<jmeterTestPlan version="1.2" properties="5.0">
  <hashTree>
    <TestPlan testname="Minimal" enabled="true"/>
    <hashTree>
      <ThreadGroup testname="TG1" enabled="true">
        <stringProp name="ThreadGroup.num_threads">5</stringProp>
        <stringProp name="ThreadGroup.ramp_time">0</stringProp>
        <stringProp name="ThreadGroup.duration">30</stringProp>
      </ThreadGroup>
      <hashTree>
        <HTTPSamplerProxy testname="GET Home" enabled="true">
          <stringProp name="HTTPSampler.method">GET</stringProp>
          <stringProp name="HTTPSampler.domain">localhost</stringProp>
          <stringProp name="HTTPSampler.protocol">http</stringProp>
          <stringProp name="HTTPSampler.path">/</stringProp>
        </HTTPSamplerProxy>
        <hashTree/>
      </hashTree>
    </hashTree>
  </hashTree>
</jmeterTestPlan>`

	result, err := ImportJMX(strings.NewReader(jmx))
	if err != nil {
		t.Fatalf("import minimal: %v", err)
	}

	if result.Plan.Name == "" {
		t.Fatal("expected non-empty plan name")
	}

	// Should have a scenario child
	root := result.Plan.Root
	if len(root.Children) == 0 {
		t.Fatal("expected children in root")
	}
}

func TestImportJMXInvalidXML(t *testing.T) {
	_, err := ImportJMX(strings.NewReader("not xml at all"))
	if err == nil {
		t.Fatal("expected error for invalid XML")
	}
}

func TestImportJMXUnsupportedElements(t *testing.T) {
	jmx := `<?xml version="1.0" encoding="UTF-8"?>
<jmeterTestPlan version="1.2">
  <hashTree>
    <TestPlan testname="Test" enabled="true"/>
    <hashTree>
      <ThreadGroup testname="TG" enabled="true">
        <stringProp name="ThreadGroup.num_threads">1</stringProp>
      </ThreadGroup>
      <hashTree>
        <JSR223Sampler testname="Groovy Script" enabled="true"/>
        <hashTree/>
      </hashTree>
    </hashTree>
  </hashTree>
</jmeterTestPlan>`

	result, err := ImportJMX(strings.NewReader(jmx))
	if err != nil {
		t.Fatalf("import: %v", err)
	}

	if len(result.Log.Unsupported) == 0 {
		t.Fatal("expected unsupported elements logged")
	}

	if !strings.Contains(result.Log.Unsupported[0], "JSR223Sampler") {
		t.Fatalf("expected JSR223Sampler in unsupported, got %q", result.Log.Unsupported[0])
	}
}
