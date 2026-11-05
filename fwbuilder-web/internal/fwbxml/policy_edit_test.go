package fwbxml
import ("strings";"testing")
func TestPolicyEdit(t *testing.T){
 d,err:=Parse(strings.NewReader(example));if err!=nil{t.Fatal(err)}
 e:=PolicyEdit{SetID:"p1",RuleID:"r1",Action:"Deny",Direction:"Both",Disabled:"False",Comment:"test",Refs:map[string][]string{"Src":{"n1"}}}
 if err:=d.EditPolicy(e);err!=nil{t.Fatal(err)}
 r,err:=d.Rules("p1");if err!=nil||len(r)!=1||r[0].Action!="Deny"||r[0].Comment!="test"{t.Fatalf("rules=%+v err=%v",r,err)}
 e.Refs=map[string][]string{"Src":{"missing"}};if err:=d.EditPolicy(e);err==nil{t.Fatal("expected invalid reference")}
 if d.Resolve("r1").Attributes["action"]!="Deny"{t.Fatal("invalid edit changed rule")}
}
