package finder

import (
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"log"
	"os"
	"path/filepath"
	"strings"
)

func FindPrometheusMetrics(dir string) ([]Desc, error) {
	var descs []Desc

	err := filepath.Walk(dir, func(path string, info fs.FileInfo, err error) error {
		if info.IsDir() {
			return nil
		}

		if !isGoSourceCodeFile(path) {
			return nil
		}

		fileHandle, err := os.Open(path)
		if err != nil {
			log.Printf("unable to open file %s: %v\n", path, err)
			return nil
		}
		defer func() {
			_ = fileHandle.Close()
		}()

		fileSet := token.NewFileSet()
		astFile, err := parser.ParseFile(
			fileSet,
			"testfile",
			fileHandle,
			parser.ParseComments,
		)
		if err != nil {
			log.Printf("unable to parse file %s: %v\n", path, err)
			return nil
		}

		ast.Inspect(astFile, func(node ast.Node) bool {
			call, ok := node.(*ast.CallExpr)
			if !ok {
				return true
			}

			fun, ok := call.Fun.(*ast.SelectorExpr)
			if !ok {
				return true
			}

			module, ok := fun.X.(*ast.Ident)
			if !ok {
				return true
			}

			if !(module.Name == "prometheus" || module.Name == "promauto") {
				return true
			}

			switch fun.Sel.Name {
			case
				"NewGauge", "NewGaugeFunc",
				"NewCounter", "NewCounterFunc",
				"NewHistogram", "NewHistogramFunc",
				"NewSummary", "NewSummaryFunc":
				if len(call.Args) == 0 {
					return true
				}

				opts, ok := call.Args[0].(*ast.CompositeLit)
				if !ok {
					return true
				}

				desc, ok := handleDesc(
					// Metric options.
					opts.Elts,
				)
				if ok {
					desc.Type = parseMetricType(fun.Sel.Name)
					descs = append(descs, desc)
				}
			case "NewGaugeVec", "NewCounterVec", "NewHistogramVec", "NewSummaryVec":
				if len(call.Args) == 0 {
					return true
				}

				opts, ok := call.Args[0].(*ast.CompositeLit)
				if !ok {
					// A function parameter. It's not possible to find an actual name right now.
					// This is a case in which prometheus metric declaration is wrapped in the
					// helper.
					return true
				}
				labels, ok := call.Args[1].(*ast.CompositeLit)
				if !ok {
					return true
				}
				desc, ok := handleVectorDesc(
					// Metric options.
					opts.Elts,
					// Metric labels.
					labels.Elts,
				)
				if ok {
					desc.Type = parseMetricType(fun.Sel.Name)
					descs = append(descs, desc)
				}
			}

			return true
		})

		return nil
	})

	return descs, err
}

func parseMetricType(name string) string {
	switch name {
	case "NewGauge":
		return "gauge"
	case "NewGaugeVec":
		return "gauge_vector"
	case "NewGaugeFunc":
		return "gauge_func"
	case "NewCounter":
		return "counter"
	case "NewCounterVec":
		return "counter_vector"
	case "NewCounterFunc":
		return "counter_func"
	case "NewHistogram":
		return "histogram"
	case "NewHistogramVec":
		return "histogram_vector"
	case "NewHistogramFunc":
		return "histogram_func"
	case "NewSummary":
		return "summary"
	case "NewSummaryVec":
		return "summary_vector"
	case "NewSummaryFunc":
		return "summary_func"
	default:
		return "unknown"
	}
}

func isGoSourceCodeFile(path string) bool {
	if strings.HasSuffix(path, ".go") && !strings.HasSuffix(path, "_test.go") {
		return true
	}
	return false
}

type Desc struct {
	Name      string   `json:"name"`
	Namespace string   `json:"namespace"`
	Subsystem string   `json:"subsystem"`
	Help      string   `json:"help"`
	Labels    []string `json:"labels,omitempty"`
	Type      string   `json:"type"`
}

func handleDesc(opts []ast.Expr) (Desc, bool) {
	return handleVectorDesc(opts, nil)
}

func handleVectorDesc(opts []ast.Expr, labels []ast.Expr) (Desc, bool) {
	var desc Desc

	for _, elt := range opts {
		kv := elt.(*ast.KeyValueExpr)
		key := kv.Key.(*ast.Ident)
		var ok bool

		switch key.Name {
		case "Name":
			desc.Name, ok = parseKV(kv.Value)
		case "Subsystem":
			desc.Subsystem, ok = parseKV(kv.Value)
		case "Namespace":
			desc.Namespace, ok = parseKV(kv.Value)
		case "Help":
			desc.Help, ok = parseKV(kv.Value)
		}

		if !ok {
			return desc, false
		}
	}

	labelNames := make([]string, 0, len(labels))
	for _, elt := range labels {
		labelName := trimDoubleQuotes(elt.(*ast.BasicLit).Value)
		labelNames = append(labelNames, labelName)
	}

	desc.Labels = labelNames
	return desc, true
}

func parseKV(kv ast.Expr) (string, bool) {
	switch namespace := kv.(type) {
	case *ast.BasicLit:
		return trimDoubleQuotes(namespace.Value), true
	case *ast.Ident:
		switch namespaceDecl := namespace.Obj.Decl.(type) {
		case *ast.ValueSpec:
			name := namespaceDecl.Values[0].(*ast.BasicLit)
			return trimDoubleQuotes(name.Value), true
		case *ast.Field:
			// A function parameter. It's not possible to find an actual name right now.
			// This is a case in which prometheus metric declaration is wrapped in the
			// helper.
			return "", false
		default:
			return "", false
		}
	default:
		return "", false
	}
}

func trimDoubleQuotes(s string) string {
	s = strings.TrimPrefix(s, `"`)
	s = strings.TrimSuffix(s, `"`)
	return s
}
