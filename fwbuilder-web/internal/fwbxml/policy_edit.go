package fwbxml

import (
 "errors"
 "fmt"
 "strings"
)

type PolicyEdit struct {
 Revision uint64 `json:"revision"`
 SetID string `json:"set_id"`
 RuleID string `json:"rule_id"`
 Move int `json:"move,omitempty"`
 Action string `json:"action,omitempty"`
 Direction string `json:"direction,omitempty"`
 Disabled string `json:"disabled,omitempty"`
 Comment string `json:"comment,omitempty"`
 Refs map[string][]string `json:"refs,omitempty"`
}

func contains(items []string,s string)bool{for _,v:=range items{if v==s{return true}};return false}

func (d *Document) EditPolicy(e PolicyEdit)error{
 set:=d.Resolve(e.SetID)
 if set==nil||set.Type!="Policy"{return errors.New("policy not found")}
 idx:=-1
 for i,c:=range set.Children{if c.ID==e.RuleID&&c.Type=="PolicyRule"{idx=i;break}}
 if idx<0{return errors.New("policy rule not found")}
 if e.Move!=0{
  if e.Move!=1&&e.Move!=-1{return errors.New("move must be -1 or 1")}
  target:=idx+e.Move
  if target<0||target>=len(set.Children)||set.Children[target].Type!="PolicyRule"{return errors.New("cannot move beyond rule set")}
  set.Children[idx],set.Children[target]=set.Children[target],set.Children[idx]
  for i,c:=range set.Children{if c.Type=="PolicyRule"{c.Attributes["position"]=fmt.Sprint(i)}}
  return nil
 }
 if !contains([]string{"Accept","Deny","Reject","Continue","Accounting","Return","Branch","Custom","Modify","Pipe","Scrub","Skip"},e.Action){return errors.New("invalid action")}
 if !contains([]string{"Both","Inbound","Outbound"},e.Direction){return errors.New("invalid direction")}
 if !contains([]string{"True","False"},e.Disabled){return errors.New("invalid disabled value")}
 if len(e.Comment)>4096{return errors.New("comment too long")}
 for key,ids:=range e.Refs{
  types:=map[string][]string{
   "Src":{"IPv4","IPv6","Network","NetworkIPv6","Host","Firewall","AddressRange","DNSName","ObjectGroup","AnyNetwork"},
   "Dst":{"IPv4","IPv6","Network","NetworkIPv6","Host","Firewall","AddressRange","DNSName","ObjectGroup","AnyNetwork"},
   "Srv":{"TCPService","UDPService","IPService","ICMPService","ICMP6Service","ServiceGroup","CustomService","AnyIPService"},
  }
  allowed,ok:=types[key];if !ok{return fmt.Errorf("unsupported rule element %s",key)}
  if len(ids)>512{return errors.New("too many references")}
  for _,id:=range ids{
   n:=d.Resolve(id);if n==nil||!contains(allowed,n.Type){return fmt.Errorf("invalid %s reference %q",key,id)}
  }
 }
 rule:=set.Children[idx]
 rule.Attributes["action"]=e.Action
 rule.Attributes["direction"]=e.Direction
 rule.Attributes["disabled"]=e.Disabled
 rule.Attributes["comment"]=e.Comment
 for key,ids:=range e.Refs{
  var el *Node
  for _,c:=range rule.Children{if c.Type==key{el=c;break}}
  if el==nil{el=&Node{Type:key,Attributes:map[string]string{"neg":"False"}};rule.Children=append(rule.Children,el)}
  kept:=make([]*Node,0,len(el.Children))
  for _,c:=range el.Children{if c.Type!="ObjectRef"&&c.Type!="ServiceRef"{kept=append(kept,c)}}
  refType:="ObjectRef";if key=="Srv"{refType="ServiceRef"}
  for _,id:=range ids{kept=append(kept,&Node{Type:refType,Attributes:map[string]string{"ref":id}})}
  el.Children=kept
 }
 return nil
}

func (d *Document) PolicyRuleCount(id string)int{
 set:=d.Resolve(id);if set==nil{return 0};count:=0
 for _,c:=range set.Children{if strings.HasSuffix(c.Type,"Rule"){count++}}
 return count
}
