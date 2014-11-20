package apptrace

import (
	"errors"
	"log"
	"sync"
)

// A Store stores and retrieves spans.
type Store interface {
	Collector

	// Trace gets a trace (a tree of spans) given its trace ID. If no
	// such trace exists, ErrTraceNotFound is returned.
	Trace(ID) (*Trace, error)
}

var (
	// ErrTraceNotFound is returned by Store.GetTrace when no trace is
	// found with the given ID.
	ErrTraceNotFound = errors.New("trace not found")
)

// A Queryer indexes spans and makes them queryable.
type Queryer interface {
	// Traces returns an implementation-defined list of traces. It is
	// a placeholder method that will be removed when other, more
	// useful methods are added to Queryer.
	Traces() ([]*Trace, error)
}

// NewMemoryStore creates a new in-memory store
func NewMemoryStore() Store {
	return &memoryStore{
		trace: map[ID]*Trace{},
		span:  map[ID]map[ID]*Trace{},
	}
}

type memoryStore struct {
	trace map[ID]*Trace        // trace ID -> trace tree
	span  map[ID]map[ID]*Trace // trace ID -> span ID -> trace (sub)tree

	sync.Mutex // protects trace

	log bool
}

// Compile-time "implements" check.
var _ interface {
	Store
	Queryer
} = &memoryStore{}

func (ms *memoryStore) Collect(id SpanID, as ...Annotation) error {
	ms.Lock()
	defer ms.Unlock()

	if ms.log {
		log.Printf("Collect %v", id)
	}

	// Initialize span map if needed.
	if _, present := ms.span[id.Trace]; !present {
		ms.span[id.Trace] = map[ID]*Trace{}
	}

	// Create or update span.
	s, present := ms.span[id.Trace][id.Span]
	if !present {
		s = &Trace{Span: Span{ID: id, Annotations: as}}
		ms.span[id.Trace][id.Span] = s
	} else {
		if ms.log {
			if len(as) > 0 {
				log.Printf("Add %d annotations to %v", len(as), id)
			}
		}
		s.Annotations = append(s.Annotations, as...)
	}

	// Create trace tree if it doesn't already exist.
	root, present := ms.trace[id.Trace]
	if !present {
		// Root span hasn't been seen yet, so make this the temporary
		// root (until we collect the actual root).
		if ms.log {
			if id.IsRoot() {
				log.Printf("Create trace %v root %v", id.Trace, id)
			} else {
				log.Printf("Create temporary trace %v root %v", id.Trace, id)
			}
		}
		ms.trace[id.Trace] = s
		root = s
	}

	// If there's a temp root and we just collected the real
	// root, fix up the tree. Or if we're the temp root's
	// parents, set us up as the new temp root.
	if isRoot, isTempRootParent := id.IsRoot(), root.Span.ID.Parent == id.Span; s != root && (isRoot || isTempRootParent) {
		oldRoot := root
		root = s
		if ms.log {
			if isRoot {
				log.Printf("Set real root %v and move temp root %v", root.Span.ID, oldRoot.Span.ID)
			} else {
				log.Printf("Set new temp root %v and move previous temp root %v (child of new temp root)", root.Span.ID, oldRoot.Span.ID)
			}
		}
		ms.trace[id.Trace] = root // set new root
		ms.reattachChildren(root, oldRoot)
		ms.insert(root, oldRoot) // reinsert the old root

		// Move the old temp root's temp children to the new
		// (possibly temp) root.
		var sub2 []*Trace
		for _, c := range oldRoot.Sub {
			if c.Span.ID.Parent != oldRoot.Span.ID.Span {
				if ms.log {
					log.Printf("Move %v from old root %v to new (possibly temp) root %v", c.Span.ID, oldRoot.Span.ID, root.Span.ID)
				}
				root.Sub = append(root.Sub, c)
			} else {
				sub2 = append(sub2, c)
			}
		}
		oldRoot.Sub = sub2
	}

	// Insert into trace tree. (We inserted the trace root span
	// above.)
	if !id.IsRoot() && s != root {
		ms.insert(root, s)
	}

	// See if we're the parent of any of the root's temporary
	// children.
	if s != root {
		ms.reattachChildren(s, root)
	}

	return nil
}

// insert inserts t into the trace tree whose root (or temp root) is
// root.
func (ms *memoryStore) insert(root, t *Trace) {
	p, present := ms.span[t.ID.Trace][t.ID.Parent]
	if present {
		if ms.log {
			log.Printf("Add %v as a child of parent %v", t.Span.ID, p.Span.ID)
		}
		p.Sub = append(p.Sub, t)
	} else {
		// Add as temporary child of the root for now. When the
		// real parent is added, we'll fix it up later.
		if ms.log {
			log.Printf("Add %v as a temporary child of root %v", t.Span.ID, root.Span.ID)
		}
		root.Sub = append(root.Sub, t)
	}
}

// reattachChildren moves temporary children of src to dst, if dst is
// the node's parent.
func (ms *memoryStore) reattachChildren(dst, src *Trace) {
	if dst == src {
		panic("dst == src")
	}
	var sub2 []*Trace
	for _, c := range src.Sub {
		if c.Span.ID.Parent == dst.Span.ID.Span {
			if ms.log {
				log.Printf("Move %v from src %v to dst %v", c.Span.ID, src.Span.ID, dst.Span.ID)
			}
			dst.Sub = append(dst.Sub, c)
		} else {
			sub2 = append(sub2, c)
		}
	}
	src.Sub = sub2
}

func (ms *memoryStore) Trace(id ID) (*Trace, error) {
	ms.Lock()
	defer ms.Unlock()

	t, present := ms.trace[id]
	if !present {
		return nil, ErrTraceNotFound
	}
	return t, nil
}

func (ms *memoryStore) Traces() ([]*Trace, error) {
	ms.Lock()
	defer ms.Unlock()

	var ts []*Trace
	for id := range ms.trace {
		t, err := ms.Trace(id)
		if err != nil {
			return nil, err
		}
		ts = append(ts, t)
	}
	return ts, nil
}