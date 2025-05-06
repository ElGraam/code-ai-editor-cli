package domain

import (
	"context"
	"go/ast"
	"go/parser"
	"go/token"
	"os"

	"github.com/google/uuid"
)

// CodeParser defines the interface for parsing code files into snippets.
type CodeParser interface {
	Parse(ctx context.Context, filepath string) ([]Snippet, error)
}

// GoCodeParser implements CodeParser for Go files.
type GoCodeParser struct{}

// NewGoCodeParser creates a new GoCodeParser.
func NewGoCodeParser() *GoCodeParser {
	return &GoCodeParser{}
}

// Parse reads a Go file, parses it, and extracts function/method declarations as Snippets.
func (p *GoCodeParser) Parse(ctx context.Context, filepath string) ([]Snippet, error) {
	content, err := os.ReadFile(filepath)
	if err != nil {
		return nil, err
	}

	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, filepath, content, parser.ParseComments)
	if err != nil {
		// Consider logging or handling specific parse errors
		return nil, err
	}

	var snippets []Snippet

	ast.Inspect(file, func(n ast.Node) bool {
		switch decl := n.(type) {
		case *ast.FuncDecl: // Includes functions and methods
			startPos := fset.Position(decl.Pos())
			endPos := fset.Position(decl.End())

			// Extract content based on byte offsets for accuracy
			startOffset := startPos.Offset
			endOffset := endPos.Offset
			if startOffset < 0 || endOffset < startOffset || endOffset > len(content) {
				// Handle invalid offsets (log or skip)
				return true // Continue inspection
			}
			codeContent := string(content[startOffset:endOffset])

			// Generate a proper UUID for the snippet instead of using filepath:pos
			id := uuid.New().String()

			symbolName := decl.Name.Name
			// Handle methods (receiver type)
			if decl.Recv != nil && len(decl.Recv.List) > 0 {
				if starExpr, ok := decl.Recv.List[0].Type.(*ast.StarExpr); ok {
					if ident, ok := starExpr.X.(*ast.Ident); ok {
						symbolName = ident.Name + "." + symbolName
					}
				} else if ident, ok := decl.Recv.List[0].Type.(*ast.Ident); ok {
					symbolName = ident.Name + "." + symbolName
				}
			}

			snippets = append(snippets, Snippet{
				ID:        id,
				Content:   codeContent,
				FilePath:  filepath,
				StartLine: startPos.Line,
				EndLine:   endPos.Line,
				Symbols:   []string{symbolName},
				// Embedding will be added later
			})
		}
		return true // Continue inspecting the AST
	})

	return snippets, nil
}
