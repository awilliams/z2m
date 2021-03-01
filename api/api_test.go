package api

import (
	"encoding/json"
	"os"
	"testing"
)

func TestGetNodesResp(t *testing.T) {
	fd, err := os.Open("testdata/getNodes.json")
	if err != nil {
		t.Fatal(err)
	}
	defer fd.Close()

	var resp GetNodesResp
	if err := json.NewDecoder(fd).Decode(&resp); err != nil {
		t.Fatal(err)
	}

	for _, n := range resp.Result {
		for _, v := range n.Values {
			if _, err := v.Value(); err != nil {
				t.Errorf("Value() error: %v", err)
			}
		}
	}
}

func TestNode_UnmarshalJSON(t *testing.T) {
	fd, err := os.Open("testdata/node.json")
	if err != nil {
		t.Fatal(err)
	}
	defer fd.Close()

	var n Node
	if err := json.NewDecoder(fd).Decode(&n); err != nil {
		t.Fatal(err)
	}
}
