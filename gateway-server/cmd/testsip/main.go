package main
import (
  "fmt"
  "siptunnel/internal/protocol/siptext"
)
func main(){
 m:=siptext.NewRequest("REGISTER","sip:34020000002000000001")
 m.SetHeader("Via","SIP/2.0/UDP siptunnel.local;branch=z9hG4bK-abc")
 m.SetHeader("From","<sip:upper@siptunnel.local>;tag=up")
 m.SetHeader("To","<sip:lower@siptunnel.local>")
 m.SetHeader("Call-ID","register-1")
 m.SetHeader("CSeq","1 REGISTER")
 m.SetHeader("Max-Forwards","70")
 m.SetHeader("Contact","<sip:10.0.0.1:5060>")
 b:=m.Bytes(); p,err:=siptext.Parse(b)
 fmt.Printf("parse=%v err=%v cseq=%s\n", p!=nil, err, p.Header("CSeq"))
}
