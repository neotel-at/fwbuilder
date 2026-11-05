package fwbxml

import (
 "strings"
 "testing"
)

const example = `<?xml version="1.0"?>
<!DOCTYPE FWObjectDatabase SYSTEM "fwbuilder.dtd">
<FWObjectDatabase xmlns="http://www.fwbuilder.org/1.0/" version="24" id="root">
<Library id="l1" name="User"><Network id="n1" name="LAN" address="10.0.0.0" netmask="255.255.255.0"/>
<Firewall id="f1" name="fw"><Policy id="p1" name="Policy"><PolicyRule id="r1" action="Accept" direction="Inbound"><Src><ObjectRef ref="n1"/></Src></PolicyRule></Policy></Firewall></Library></FWObjectDatabase>`

func TestParseAndRules(t *testing.T){
 d,err:=Parse(strings.NewReader(example));if err!=nil{t.Fatal(err)}
 if d.Version!="24"||d.Count!=8 {t.Fatalf("unexpected document %+v",d)}
 if d.Resolve("n1").Name!="LAN"{t.Fatal("object index failed")}
 rules,err:=d.Rules("p1");if err!=nil{t.Fatal(err)}
 if len(rules)!=1||rules[0].Action!="Accept"||rules[0].Elements["Src"][0]!="LAN"{t.Fatalf("unexpected rules %+v",rules)}
}
func TestRejectDuplicateIDs(t *testing.T){
 _,err:=Parse(strings.NewReader(`<FWObjectDatabase><Library id="x"/><Library id="x"/></FWObjectDatabase>`))
 if err==nil{t.Fatal("expected duplicate ID error")}
}
func TestRejectWrongRoot(t *testing.T){
 _,err:=Parse(strings.NewReader(`<root/>`));if err==nil{t.Fatal("expected error")}
}
