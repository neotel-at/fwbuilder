package fwbxml

import (
    "bytes"
    "encoding/xml"
    "errors"
    "fmt"
    "io"
    "strings"
)

const MaxDocumentSize int64 = 32 << 20
const MaxNodes = 250000

type Node struct {
    Type string `json:"type"`
    ID string `json:"id,omitempty"`
    Name string `json:"name,omitempty"`
    Attributes map[string]string `json:"attributes,omitempty"`
    Children []*Node `json:"children,omitempty"`
    Text string `json:"text,omitempty"`
}

type Document struct {
    Version string `json:"version"`
    Root *Node `json:"root"`
    Index map[string]*Node `json:"-"`
    Count int `json:"count"`
}

func Parse(r io.Reader) (*Document, error) {
    dec := xml.NewDecoder(io.LimitReader(r, MaxDocumentSize+1))
    dec.Strict = true
    doc := &Document{Index: make(map[string]*Node)}
    var stack []*Node
    for {
        tok, err := dec.Token()
        if errors.Is(err, io.EOF) { break }
        if err != nil { return nil, fmt.Errorf("invalid XML: %w", err) }
        switch t := tok.(type) {
        case xml.StartElement:
            doc.Count++
            if doc.Count > MaxNodes { return nil, fmt.Errorf("node limit exceeded (%d)", MaxNodes) }
            n := &Node{Type:t.Name.Local, Attributes: make(map[string]string)}
            for _, a := range t.Attr {
                key := a.Name.Local
                if a.Name.Space != "" && a.Name.Space != "xmlns" { key = a.Name.Space+":"+key }
                n.Attributes[key] = a.Value
            }
            n.ID, n.Name = n.Attributes["id"], n.Attributes["name"]
            if len(stack) == 0 {
                if doc.Root != nil || n.Type != "FWObjectDatabase" { return nil, errors.New("expected FWObjectDatabase root") }
                doc.Root = n
                doc.Version = n.Attributes["version"]
            } else {
                parent := stack[len(stack)-1]
                parent.Children = append(parent.Children,n)
            }
            if n.ID != "" {
                if _, exists := doc.Index[n.ID]; exists { return nil, fmt.Errorf("duplicate object id %q", n.ID) }
                doc.Index[n.ID] = n
            }
            stack = append(stack,n)
        case xml.EndElement:
            if len(stack)==0 { return nil, errors.New("unexpected closing element") }
            stack = stack[:len(stack)-1]
        case xml.CharData:
            if len(stack)>0 {
                s := strings.TrimSpace(string(bytes.TrimSpace(t)))
                if s!="" {
                    node := stack[len(stack)-1]
                    if len(node.Text)+len(s) <= 8192 { node.Text += s }
                }
            }
        case xml.Directive:
            // Go's decoder does not fetch external DTDs, and no entity expansion is enabled.
            // Legacy .fwb files may declare <!DOCTYPE FWObjectDatabase SYSTEM "fwbuilder.dtd">.
            if bytes.HasPrefix(bytes.ToUpper(bytes.TrimSpace(t)), []byte("DOCTYPE")) { continue }
        }
    }
    if doc.Root == nil || len(stack)>0 { return nil, errors.New("incomplete FWObjectDatabase") }
    return doc,nil
}

func (d *Document) Resolve(id string) *Node { return d.Index[id] }

type Rule struct {
    ID string `json:"id"`
    Name string `json:"name"`
    Type string `json:"type"`
    Action string `json:"action,omitempty"`
    Direction string `json:"direction,omitempty"`
    Disabled string `json:"disabled,omitempty"`
    Comment string `json:"comment,omitempty"`
    Elements map[string][]string `json:"elements"`
    Details map[string][]RuleObject `json:"details"`
    Options bool `json:"options"`
}

type RuleObject struct {
 ID string `json:"id"`
 Name string `json:"name"`
 Type string `json:"type"`
}

func (d *Document) Rules(ruleSetID string) ([]Rule,error) {
    set := d.Resolve(ruleSetID)
    if set==nil { return nil, errors.New("rule set not found") }
    if set.Type!="Policy" && set.Type!="NAT" && set.Type!="Routing" { return nil, errors.New("object is not a rule set") }
    result := make([]Rule,0)
    for _, child := range set.Children {
        if !strings.HasSuffix(child.Type,"Rule") { continue }
        rule := Rule{ID:child.ID,Name:child.Name,Type:child.Type,Action:child.Attributes["action"],Direction:child.Attributes["direction"],Disabled:child.Attributes["disabled"],Comment:child.Attributes["comment"],Elements:make(map[string][]string),Details:make(map[string][]RuleObject)}
        for _, el := range child.Children {
            if el.Type=="PolicyRuleOptions" || el.Type=="NATRuleOptions" {rule.Options=true}
            for _, ref := range el.Children {
                if ref.Type!="ObjectRef" && ref.Type!="ServiceRef" && ref.Type!="IntervalRef" { continue }
                target := d.Resolve(ref.Attributes["ref"])
                label := ref.Attributes["ref"]
                if target != nil {
                    label = target.Name
                    if label=="" { label=target.Type }
                }
                rule.Elements[el.Type] = append(rule.Elements[el.Type],label)
                kind:="Unknown"
                if target!=nil {kind=target.Type}
                rule.Details[el.Type]=append(rule.Details[el.Type],RuleObject{ID:ref.Attributes["ref"],Name:label,Type:kind})
            }
        }
        result=append(result,rule)
    }
    return result,nil
}
