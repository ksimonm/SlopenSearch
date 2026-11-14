package mapping

import (
	"strings"

	"slopensearch/internal/jsonv"
)

var KnnFieldParams = map[string]bool{
	"type": true, "dimension": true, "data_type": true, "space_type": true, "method": true,
	"mode": true, "compression_level": true, "doc_values": true, "model_id": true,
}

type KnnFieldInfo struct {
	Space, Engine, DataType string
	Dim                     int
	nested                  bool
}

func KnnInfo(def map[string]any) KnnFieldInfo {
	fi := KnnFieldInfo{Space: "l2", Engine: "faiss", DataType: "float"}
	if f, ok := jsonv.ToFloat(def["dimension"]); ok {
		fi.Dim = int(f)
	}
	if dt, ok := def["data_type"].(string); ok {
		fi.DataType = strings.ToLower(dt)
	}
	if fi.DataType == "binary" {
		fi.Space = "hamming"
	}
	if m, ok := def["method"].(map[string]any); ok {
		if e, ok := m["engine"].(string); ok {
			fi.Engine = e
		}
		if s, ok := m["space_type"].(string); ok {
			fi.Space = s
		}
	}
	if s, ok := def["space_type"].(string); ok && s != "" {
		fi.Space = s
	}
	return fi
}
