package tools

import (
	"context"
	"fmt"
	"testing"
)

func TestDDGTool_Execute(t *testing.T) {
	tool := NewDuckDuckGoTool()
	ctx := context.Background()
	args := `{"query": "happyhorse是谁家的产品？"}`
	result, err := tool.Execute(ctx, args)
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}
	fmt.Println("DDGTool Execute result:", result)

}
