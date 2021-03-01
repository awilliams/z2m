// Package z2m is a client for the zwavejs2mqtt project's MQTT API.
//
// https://zwave-js.github.io/zwavejs2mqtt/#/
package z2m

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"path"
	"sync"

	"github.com/awilliams/z2m/api"
)

// NewBroker returns a Broker instance that uses
// the given Publisher to publish messages to the zwavejs2mqtt
// API.
func NewBroker(publisher Publisher) *Broker {
	return &Broker{
		p:           publisher,
		gotNodes:    make(chan error),
		setAttr:     make(map[api.ValueID]chan<- error),
		nodesByName: make(map[string]*Node),
		nodesByID:   make(map[int]*Node),
	}
}

// Broker sends and receives messages from the zwavejs2mqtt API.
type Broker struct {
	p Publisher

	gotNodes chan error

	mu      sync.RWMutex // Protects following
	setAttr map[api.ValueID]chan<- error

	nodesByName map[string]*Node // Node.Name -> Node
	nodesByID   map[int]*Node    // Node.ID -> Node
}

// Subscriptions returns a map of MQTT topics that should be subscribed to
// and their corresponding handler functions.
func (b *Broker) Subscriptions(topicPrefix string) map[string]func(payload []byte) error {
	return map[string]func([]byte) error{
		path.Join(topicPrefix, api.TopicGetNodesResp):         b.handleGetNodesResp,
		path.Join(topicPrefix, api.TopicWriteValueResp):       b.handleWriteValueResp,
		path.Join(topicPrefix, api.TopicNodeValueUpdateEvent): b.handleNodeValueUpdated,
	}
}

func (b *Broker) WatchValue(nodeName, property string, v chan<- interface{}) (func(), error) {
	// TODO: Send nodeName and property along with value on channel. This
	// would allow a single channel to be used for multiple watches.
	b.mu.Lock()
	defer b.mu.Unlock()

	n, ok := b.nodesByName[nodeName]
	if !ok {
		return nil, fmt.Errorf("node %q not found", nodeName)
	}
	if _, ok := n.valuesByProperty[property]; !ok {
		return nil, fmt.Errorf("node:%d does not have property %q", n.ID, property)
	}

	w, ok := n.valueWatchers[property]
	if !ok {
		w = make(map[chan<- interface{}]struct{})
		n.valueWatchers[property] = w
	}

	w[v] = struct{}{}
	cancel := func() {
		b.mu.Lock()
		defer b.mu.Unlock()
		delete(w, v)
	}
	return cancel, nil
}

// SettAttr updates a given node's attribute. The method blocks until a response is received
// or the context timesout, whichever comes first.
func (b *Broker) SetAttr(ctx context.Context, nodeName, property string, value interface{}) error {
	b.mu.Lock()

	n, ok := b.nodesByName[nodeName]
	if !ok {
		b.mu.Unlock()
		return fmt.Errorf("node %q not found", nodeName)
	}

	v, ok := n.valuesByProperty[property]
	if !ok {
		b.mu.Unlock()
		return fmt.Errorf("node %q has no attribute with label %q", nodeName, property)
	}

	msg := v.WriteValue(value)
	vid, ok := msg.Args[0].(api.ValueID)
	if !ok {
		b.mu.Unlock()
		return fmt.Errorf("unexpected arg[0] type %T", msg.Args[0])
	}

	payload, err := json.Marshal(msg)
	if err != nil {
		return err
	}

	wait := make(chan error, 1)
	b.setAttr[vid] = wait
	b.mu.Unlock()

	defer func() {
		b.mu.Lock()
		delete(b.setAttr, vid)
		b.mu.Unlock()
	}()

	if err := b.p.Publish("_CLIENTS/ZWAVE_GATEWAY/api/writeValue/set", payload); err != nil {
		return err
	}

	select {
	case <-ctx.Done():
		return ctx.Err()
	case err := <-wait:
		return err
	}
}

func (b *Broker) GetNodes(ctx context.Context) error {
	if err := b.p.Publish(api.TopicGetNodesReq, []byte(nil)); err != nil {
		return err
	}
	if ctx == nil {
		return nil
	}

	select {
	case <-ctx.Done():
		return ctx.Err()
	case err := <-b.gotNodes:
		return err
	}
}

func (b *Broker) handleNodeValueUpdated(payload []byte) error {
	var update api.NodeValueUpdate
	if err := json.Unmarshal(payload, &update); err != nil {
		return err
	}

	val, err := update.Value.Value()
	if err != nil {
		return err
	}

	b.mu.RLock()
	defer b.mu.RUnlock()

	n, ok := b.nodesByID[update.NodeID]
	if !ok {
		return nil
	}

	cbs, ok := n.valueWatchers[update.Property.String()]
	if !ok {
		return nil
	}

	for c := range cbs {
		select {
		case c <- val:
		default:
		}
	}

	return nil
}

func (b *Broker) handleWriteValueResp(payload []byte) error {
	// {"success":true,"message":"Success zwave api call","args":[{"nodeId":4,"commandClass":38,"endpoint":0,"property":"targetValue"},93]}
	var resp api.WriteValueResp
	if err := json.Unmarshal(payload, &resp); err != nil {
		return err
	}

	b.mu.Lock()
	defer b.mu.Unlock()

	wait, ok := b.setAttr[resp.ValueID]
	if !ok {
		return nil
	}

	if !resp.Success {
		err := fmt.Errorf("write value response not successful: %s", resp.Message)
		wait <- err
		return err
	}

	wait <- nil
	return nil
}

func (b *Broker) handleGetNodesResp(payload []byte) error {
	err := func() error {
		var resp api.GetNodesResp
		if err := json.Unmarshal(payload, &resp); err != nil {
			return fmt.Errorf("unable to parse GetNodes response: %w", err)
		}
		if !resp.Success {
			return errors.New("unsuccessful GetNodes response")
		}

		b.mu.Lock()
		defer b.mu.Unlock()

		// Clear existing nodes map, since this is only expected one time
		// at startup.
		for k := range b.nodesByName {
			delete(b.nodesByName, k)
		}
		for k := range b.nodesByID {
			delete(b.nodesByID, k)
		}

		for _, n := range resp.Result {
			if n.Failed {
				// Ignore dead or removed nodes.
				continue
			}
			if _, dup := b.nodesByName[n.Name]; dup {
				return fmt.Errorf("duplicate node name %q", n.Name)
			}

			node := Node{
				Node:             n,
				valuesByProperty: make(map[string]*api.Value, len(n.Values)),
				valuesByID:       make(map[string]*api.Value, len(n.Values)),
				valueWatchers:    make(map[string]map[chan<- interface{}]struct{}),
			}
			for _, v := range n.Values {
				v := v
				// TODO: check for duplicates.
				node.valuesByProperty[v.Property.String()] = &v
				node.valuesByID[v.ID] = &v
			}

			if n.Name != "" {
				b.nodesByName[n.Name] = &node
			}
			b.nodesByID[n.ID] = &node
		}
		return nil
	}()

	select {
	case b.gotNodes <- err:
	default:
	}

	return err
}

type Node struct {
	api.Node

	valuesByProperty map[string]*api.Value // Value.Property -> api.Value
	valuesByID       map[string]*api.Value // Value.ID -> api.Value
	valueWatchers    map[string]map[chan<- interface{}]struct{}
}
