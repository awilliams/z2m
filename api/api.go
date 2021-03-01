// Package api defines types for use with the ZWave2MQTT MQTT API.
package api

import (
	"encoding/json"
	"fmt"
	"strconv"
	"time"
)

const (
	// The documentation lists this topic as:
	//   <mqtt_prefix>/_CLIENTS/ZWAVE_GATEWAY-<mqtt_name>/api/<api_name>/set
	// The <mqtt_name> portion doesn't seem to be necessary.
	// https://github.com/OpenZWave/Zwave2Mqtt#zwave-apis
	TopicGetNodesReq          = "/_CLIENTS/ZWAVE_GATEWAY/api/getNodes/set"
	TopicGetNodesResp         = "/_CLIENTS/ZWAVE_GATEWAY/api/getNodes"
	TopicWriteValueResp       = "/_CLIENTS/ZWAVE_GATEWAY/api/writeValue"
	TopicNodeValueUpdateEvent = "/_EVENTS/+/node/node_value_updated"

	// Various Value "types".
	// https://github.com/zwave-js/node-zwave-js/blob/fa1bbf556860665d396d4801a412b45e2bb72087/packages/core/src/values/Metadata.ts#L29-L38
	TypeNumber     = "number"
	TypeBool       = "boolean"
	TypeString     = "string"
	TypeListNumber = "number[]"
	TypeListBool   = "boolean[]"
	TypeListString = "string[]"
	TypeDuration   = "duration"
	TypeColor      = "color"
	TypeAny        = "any"

	// Various duration values.
	// https://github.com/zwave-js/node-zwave-js/blob/0a7bdb15dd50ecc5aa146c12c20b360320b9e169/packages/core/src/values/Duration.ts#L5
	DurationSeconds = "seconds"
	DurationMinutes = "minutes"
)

// GetNodesResp is the response structure of the GetNodes API.
// https://github.com/OpenZWave/Zwave2Mqtt#custom-apis
type GetNodesResp struct {
	Success bool   `json:"success"`
	Message string `json:"message"`
	Result  []Node `json:"result"`
}

// Node is a Z-Wave node.
type Node struct {
	ID                  int    `json:"id"`
	DeviceID            string `json:"deviceId"`
	Manufacturer        string `json:"manufacturer"`
	Name                string `json:"name"`
	Loc                 string `json:"loc"`
	Ready               bool   `json:"ready"`
	Available           bool   `json:"available"`
	Failed              bool   `json:"failed"`
	LastActive          int64  `json:"lastActive"`
	InterviewCompleted  bool   `json:"interviewCompleted"`
	FirmwareVersion     string `json:"firmwareVersion"`
	IsBeaming           bool   `json:"isBeaming"`
	KeepAwake           bool   `json:"keepAwake"`
	IsRouting           bool   `json:"isRouting"`
	IsFrequentListening bool   `json:"isFrequentListening"`
	IsListening         bool   `json:"isListening"`
	Status              string `json:"status"`
	InterviewStage      string `json:"interviewStage"`
	ProductLabel        string `json:"productLabel"`
	ProductDescription  string `json:"productDescription"`
	ZwaveVersion        int    `json:"zwaveVersion"`
	DeviceClass         struct {
		Basic    int `json:"basic"`
		Generic  int `json:"generic"`
		Specific int `json:"specific"`
	} `json:"deviceClass"`
	HexID  string           `json:"hexId"`
	DbLink string           `json:"dbLink"`
	Values map[string]Value `json:"values"`
}

// Value is a capability of a Z-Wave node.
type Value struct {
	ID               string    `json:"id"`
	NodeID           int       `json:"nodeId"`
	CommandClass     int       `json:"commandClass"`
	CommandClassName string    `json:"commandClassName"`
	Endpoint         int       `json:"endpoint"`
	Property         StringInt `json:"property"`
	PropertyName     string    `json:"propertyName"`
	Type             string    `json:"type"`
	Readable         bool      `json:"readable"`
	Writeable        bool      `json:"writeable"`
	Label            string    `json:"label"`
	Stateless        bool      `json:"stateless"`
	Min              int       `json:"min,omitempty"`
	Max              int       `json:"max,omitempty"`
	List             bool      `json:"list"`
	LastUpdate       int64     `json:"lastUpdate"`
	IsCurrentValue   bool      `json:"isCurrentValue"`
	TargetValue      string    `json:"targetValue"`
	Default          int       `json:"default"`
	States           []struct {
		Text  string `json:"text"`
		Value int    `json:"value"`
	} `json:"states"`
	RawValue json.RawMessage `json:"value"`
}

// Value uses the Value's Type to decode the value field
// into the proper Go type, which is then returned.
func (v Value) Value() (interface{}, error) {
	switch v.Type {
	case TypeNumber:
		var tv int
		if len(v.RawValue) > 0 {
			if err := json.Unmarshal(v.RawValue, &tv); err != nil {
				return nil, fmt.Errorf("unable to parse value of type %q: %w", v.Type, err)
			}
		}
		return tv, nil
	case TypeBool:
		var tv bool
		if len(v.RawValue) > 0 {
			if err := json.Unmarshal(v.RawValue, &tv); err != nil {
				return nil, fmt.Errorf("unable to parse value of type %q (%q): %w", string(v.RawValue), v.Type, err)
			}
		}
		return tv, nil
	case TypeString, TypeColor:
		var tv string
		if err := json.Unmarshal(v.RawValue, &tv); err != nil {
			return nil, fmt.Errorf("unable to parse value of type %q: %w", v.Type, err)
		}
		return tv, nil
	case TypeDuration:
		var unit struct {
			Unit string `json:"unit"`
		}
		if err := json.Unmarshal(v.RawValue, &unit); err != nil {
			return nil, fmt.Errorf("unable to parse value of type %q: %w", v.Type, err)
		}
		// https://github.com/zwave-js/node-zwave-js/blob/0a7bdb15dd50ecc5aa146c12c20b360320b9e169/packages/core/src/values/Duration.ts#L5
		switch unit.Unit {
		case DurationSeconds:
			return time.Second, nil
		case DurationMinutes:
			return time.Minute, nil
		default:
			return time.Duration(0), nil
		}
	case TypeAny:
		return v.RawValue, nil
	default:
		return nil, fmt.Errorf("unknown value type %q", v.Type)
	}
}

func (v Value) WriteValue(value interface{}) APIArgs {
	// TODO: It would be possible to do limited type checking
	// on value based on v.Type.
	return APIArgs{
		Args: []interface{}{
			ValueID{
				NodeID:       v.NodeID,
				CommandClass: v.CommandClass,
				Endpoint:     v.Endpoint,
				Property:     v.Property.String(),
			},
			value,
		},
	}
}

type StringInt struct {
	s string
	i int
}

func (s *StringInt) UnmarshalJSON(b []byte) error {
	if len(b) == 0 {
		return nil
	}
	if b[0] == '"' {
		return json.Unmarshal(b, &s.s)
	}
	return json.Unmarshal(b, &s.i)
}

func (s *StringInt) String() string {
	if s.s != "" {
		return s.s
	}
	return strconv.Itoa(s.i)
}

type APIArgs struct {
	Args []interface{} `json:"args"`
}

type ValueID struct {
	NodeID       int    `json:"nodeId"`
	CommandClass int    `json:"commandClass"`
	Endpoint     int    `json:"endpoint"`
	Property     string `json:"property"`
}

type WriteValueResp struct {
	Success bool
	Message string
	ValueID ValueID
	Value   json.RawMessage
}

func (w *WriteValueResp) UnmarshalJSON(b []byte) error {
	var obj struct {
		Success bool              `json:"success"`
		Message string            `json:"message"`
		Args    []json.RawMessage `json:"args"`
	}
	if err := json.Unmarshal(b, &obj); err != nil {
		return err
	}

	if l := len(obj.Args); l != 2 {
		return fmt.Errorf("unexpected args array of length %d; expected %d", l, 2)
	}

	w.Success = obj.Success
	w.Message = obj.Message

	if err := json.Unmarshal(obj.Args[0], &w.ValueID); err != nil {
		return err
	}
	w.Value = obj.Args[1]
	return nil
}

type NodeValueUpdate struct {
	Value
}

func (n *NodeValueUpdate) UnmarshalJSON(b []byte) error {
	var obj struct {
		Data []json.RawMessage `json:"data"`
	}
	if err := json.Unmarshal(b, &obj); err != nil {
		return err
	}

	if l := len(obj.Data); l != 2 {
		return fmt.Errorf("data array has length %d; expected %d", l, 2)
	}

	var node Node
	if err := json.Unmarshal(obj.Data[0], &node); err != nil {
		return err
	}

	/*
	   {
	   	"commandClassName": "Multilevel Switch",
	   	"commandClass": 38,
	   	"endpoint": 0,
	   	"property": "currentValue",
	   	"newValue": 3,
	   	"prevValue": 99,
	   	"propertyName": "currentValue"
	   }
	*/
	var update struct {
		CommandClass int       `json:"commandClass"`
		Endpoint     int       `json:"endpoint"`
		Property     StringInt `json:"property"`
		PropertyKey  string    `json:"propertyKey"`
	}
	if err := json.Unmarshal(obj.Data[1], &update); err != nil {
		return err
	}

	id := fmt.Sprintf("%d-%d-%s", update.CommandClass, update.Endpoint, update.Property.String())
	if update.PropertyKey != "" {
		id = id + "-" + update.PropertyKey
	}
	value, ok := node.Values[id]
	if !ok {
		return fmt.Errorf("unable to find update value %q", id)
	}

	n.Value = value

	return nil
}
