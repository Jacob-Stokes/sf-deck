// Package soqlauto provides context-aware autocomplete suggestions
// for SOQL editors. Behavioural parity target: Salesforce Inspector
// Reloaded's data-export.js autocomplete handler.
//
// Pure logic — no UI deps. Callers (Bubble Tea handlers, REPLs,
// future Apex anonymous editor) build a Snapshot per keystroke and
// receive a ranked []Suggestion. Describe loading is lazy: misses
// emit to Classification.LoadingFor and the caller is responsible
// for kicking off the fetch + re-invoking on completion.
//
// Implementation strategy (copied from Inspector): regex-based
// classification of the substring before the caret, with a flat
// loop over relationship hops to traverse the describe graph.
// No parser, no tokenizer.
package soqlauto

import "github.com/Jacob-Stokes/sf-deck/internal/sf"

// Snapshot is the synchronous input to Classify + Suggest. The
// caller assembles it from the editor's query buffer + the active
// org's describe cache.
type Snapshot struct {
	Query     string
	CursorPos int // byte offset
	SelEnd    int // == CursorPos when no selection
	Tooling   bool

	// Describes returns the cached describe ref for an sObject.
	// Status == Loaded means Describe is non-nil; any other status
	// means the engine should NOT walk its fields and the caller
	// should kick a fetch.
	Describes func(sobject string) DescribeRef

	// EnsureDescribe is a fire-and-forget hint that the engine
	// needs this describe loaded. The caller wires it to the
	// app's describe-fetch command; subsequent Snapshots will
	// see Status == Loaded once the fetch lands.
	EnsureDescribe func(sobject string)

	// SObjects is the list of API names for sObjects in the
	// active org. Used to suggest sObjects after FROM. Empty when
	// the catalog hasn't loaded yet.
	SObjects []string
}

// DescribeStatus reports the load state of one sObject describe.
type DescribeStatus int

const (
	StatusLoaded DescribeStatus = iota
	StatusLoading
	StatusNotFound
	StatusLoadFailed
	StatusUnknown
)

// DescribeRef pairs a load status with the describe payload (nil
// unless Status == Loaded).
type DescribeRef struct {
	Status   DescribeStatus
	Describe *sf.SObjectDescribe
}

// Classification is the engine's understanding of where the caret
// sits in the SOQL query. Drives which Suggestions kind to emit.
type Classification struct {
	Context     ContextKind
	SearchToken string   // trailing word the user is typing
	ContextPath string   // trailing dotted chain BEFORE the token
	Sobject     string   // resolved active sObject (subquery-aware)
	OperatorRHS bool     // right of =, <, >, !=, LIKE, etc.
	InSubquery  bool     // caret is inside a (SELECT ... FROM x) block
	OperatorOp  string   // the operator text when OperatorRHS — for IN(...) detection
	WhereField  string   // for WhereValue context, the field being filtered (dotted)
	LoadingFor  []string // sObjects we'd need but don't have describes for
}

// ContextKind enumerates every editor-position the classifier
// recognises. Priority order: first match wins (see context.go's
// Classify cascade).
type ContextKind int

const (
	ContextTopLevel           ContextKind = iota // empty or between clauses
	ContextAfterFromKeyword                      // right after "FROM "
	ContextAfterSelectKeyword                    // inside SELECT field list
	ContextWhereField                            // field slot in WHERE / AND / OR
	ContextWhereValue                            // value slot after operator
	ContextInWithValues                          // inside IN (...) value list
	ContextOrderByField                          // field slot in ORDER BY
	ContextGroupByField                          // field slot in GROUP BY
	ContextInSubquery                            // child-relationship subquery
	ContextNumericLiteral                        // after LIMIT / OFFSET — expects a number
)

func (c ContextKind) String() string {
	switch c {
	case ContextAfterFromKeyword:
		return "after-from"
	case ContextAfterSelectKeyword:
		return "select-fields"
	case ContextWhereField:
		return "where-field"
	case ContextWhereValue:
		return "where-value"
	case ContextInWithValues:
		return "in-values"
	case ContextOrderByField:
		return "order-by"
	case ContextGroupByField:
		return "group-by"
	case ContextInSubquery:
		return "subquery"
	default:
		return "top-level"
	}
}

// Suggestion is one entry in the popup list.
type Suggestion struct {
	Value    string         // text to insert
	Display  string         // primary text in the popup
	Detail   string         // secondary text (label, type, target sObject)
	Suffix   string         // appended after Value on accept (" ", ", ", "")
	Kind     SuggestionKind // for styling + filtering
	DataType string         // field.Type when Kind == field/relationship
	Rank     int            // higher = better
}

// SuggestionKind drives popup row styling.
type SuggestionKind int

const (
	KindField SuggestionKind = iota
	KindRelationship
	KindSObject
	KindKeyword
	KindFunction
	KindPicklist
	KindDateLiteral
	KindBoolean
	KindNull
	KindLiteral
)

func (k SuggestionKind) String() string {
	switch k {
	case KindRelationship:
		return "rel"
	case KindSObject:
		return "sobj"
	case KindKeyword:
		return "keyword"
	case KindFunction:
		return "fn"
	case KindPicklist:
		return "picklist"
	case KindDateLiteral:
		return "date"
	case KindBoolean:
		return "bool"
	case KindNull:
		return "null"
	case KindLiteral:
		return "literal"
	default:
		return "field"
	}
}
