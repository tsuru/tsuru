package godap

// our minimalistic LDAP server that translates LDAP bind requests
// into a simple callback function

import (
	"fmt"
	"github.com/go-asn1-ber/asn1-ber"
	// "io/ioutil"
	"log"
	"net"
	"time"
)

// lame, but simple - set to true when you want log output
var LDAPDebug = false

func ldapdebug(format string, v ...interface{}) {
	if LDAPDebug {
		log.Printf(format, v...)
	}
}

// Handles socket interaction and a chain of handlers
type LDAPServer struct {
	Listener net.Listener
	Handlers []LDAPRequestHandler
}

// listens and runs a plain (non-TLS) LDAP server on the address:port specified
func (s *LDAPServer) ListenAndServe(addr string) error {
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return err
	}
	s.Listener = ln
	return s.Serve()
}

// something that the handlers can use to keep track of stuff across multiple requests in the same connection/session
type LDAPSession struct {
	Attributes map[string]interface{}
}

// serves an ldap server on the listener specified in the LDAPServer struct
func (s *LDAPServer) Serve() error {
	for {
		conn, err := s.Listener.Accept()
		if err != nil {
			return err
		}
		go func(conn net.Conn) {

			// catch and report panics - we don't want it to crash the server
			defer func() {
				if r := recover(); r != nil {
					log.Printf("Caught panic while serving request (conn=%v): %v", conn, r)
				}
			}()

			defer conn.Close()

			// keep the connection around for a minute
			conn.SetDeadline(time.Now().Add(time.Minute))

			ssn :=
				&LDAPSession{
					Attributes: make(map[string]interface{}),
				}

			for {

				// read a request
				p, err := ber.ReadPacket(conn)
				if err != nil {
					ldapdebug("Error while reading packet: %v", err)
					return
				}

				handled := false
				for _, h := range s.Handlers {
					retplist := h.ServeLDAP(ssn, p)
					if len(retplist) > 0 {

						ldapdebug("Got LDAP response(s) from handler, writing it/them back to client")

						for _, retp := range retplist {

							b := retp.Bytes()

							_, err = conn.Write(b)
							if err != nil {
								ldapdebug("Error while writing response packet: %v", err)
								return
							}
						}

						handled = true
						break
					}
				}

				if !handled {

					if IsUnbindRequest(p) {
						ldapdebug("Got unbind request, closing connection")
						return
					}

					ldapdebug("Unhandled packet, closing connection: %v", p)
					if LDAPDebug {
						ber.PrintPacket(p)
						// ioutil.WriteFile("tmpdump.dat", p.Bytes(), 0644)
					}
					// TODO: in theory, we should be sending a "Notice of Disconnection"
					// here, in practice I don't know that it matters
					return
				}

				// loop back around and wait for another

			}

		}(conn)
	}
	return nil
}

// processes a request, or not
type LDAPRequestHandler interface {
	// read a packet and return one or more packets as a response
	// or nil/empty to indicate we don't want to handle this packet
	ServeLDAP(*LDAPSession, *ber.Packet) []*ber.Packet
}

type LDAPResultCodeHandler struct {
	ReplyTypeId int64 // the overall type of the response, e.g. 1 is BindResponse - it must be a response that is just a result code
	ResultCode  int64 // the result code, i.e. 0 is success, 49 is invalid credentials, etc.
}

func (h *LDAPResultCodeHandler) ServeLDAP(ssn *LDAPSession, p *ber.Packet) []*ber.Packet {

	// extract message ID...
	messageId, err := ExtractMessageId(p)
	if err != nil {
		ldapdebug("Unable to extract message id: %v", err)
		return nil
	}

	replypacket := ber.Encode(ber.ClassUniversal, ber.TypeConstructed, ber.TagSequence, nil, "LDAP Response")
	replypacket.AppendChild(ber.NewInteger(ber.ClassUniversal, ber.TypePrimitive, ber.TagInteger, messageId, "MessageId"))
	bindResult := ber.Encode(ber.ClassApplication, ber.TypeConstructed, ber.Tag(h.ReplyTypeId), nil, "Response")
	bindResult.AppendChild(ber.NewInteger(ber.ClassUniversal, ber.TypePrimitive, ber.TagEnumerated, h.ResultCode, "Result Code"))
	// per the spec these are "matchedDN" and "diagnosticMessage", but we don't need them for this
	bindResult.AppendChild(ber.NewString(ber.ClassUniversal, ber.TypePrimitive, ber.TagOctetString, "", "Unused"))
	bindResult.AppendChild(ber.NewString(ber.ClassUniversal, ber.TypePrimitive, ber.TagOctetString, "", "Unused"))
	replypacket.AppendChild(bindResult)

	return []*ber.Packet{replypacket}

}

// function that checks simple auth credentials (username/password style)
type LDAPBindFunc func(binddn string, bindpw []byte) bool

// responds to bind requests
type LDAPBindFuncHandler struct {
	LDAPBindFunc LDAPBindFunc
}

func (h *LDAPBindFuncHandler) ServeLDAP(ssn *LDAPSession, p *ber.Packet) []*ber.Packet {

	reth := &LDAPResultCodeHandler{ReplyTypeId: 1, ResultCode: 49}

	// this is really just part of the message validation - we don't need the message id here
	_, err := ExtractMessageId(p)
	if err != nil {
		ldapdebug("Unable to extract message id: %v", err)
		return nil
	}

	// check for bind request contents
	err = CheckPacket(p.Children[1], ber.ClassApplication, ber.TypeConstructed, 0x0)
	if err != nil {
		//ldapdebug("Not a bind request: %v", err)
		return nil
	}

	// this should be ldap version
	err = CheckPacket(p.Children[1].Children[0], ber.ClassUniversal, ber.TypePrimitive, ber.TagInteger)
	if err != nil {
		ldapdebug("Error trying to read the ldap version: %v", err)
		return nil
	}

	// check ldap version number
	ldapv := ForceInt64(p.Children[1].Children[0].Value)
	if ldapv < 2 {
		ldapdebug("LDAP version too small - should be >= 2 but was: %v", ldapv)
		return nil
	}

	// make sure we have at least our version number, bind dn and bind password
	if len(p.Children[1].Children) < 3 {
		ldapdebug("At least 3 elements required in bind request, found %v", len(p.Children[1].Children))
		return nil
	}

	// should be the bind DN (the "username") - note that this will fail if it's SASL auth
	err = CheckPacket(p.Children[1].Children[1], ber.ClassUniversal, ber.TypePrimitive, ber.TagOctetString)
	if err != nil {
		ldapdebug("Error verifying packet: %v", err)
		return nil
	}
	myBindDn := string(p.Children[1].Children[1].ByteValue)
	// fmt.Printf("myBindDn: %v\n", myBindDn)

	err = CheckPacket(p.Children[1].Children[2], ber.ClassContext, ber.TypePrimitive, 0x0)
	if err != nil {
		ldapdebug("Error verifying packet: %v", err)
		return nil
	}

	myBindPw := p.Children[1].Children[2].Data.Bytes()
	// fmt.Printf("myBindPw: %v\n", myBindPw)

	// call back to the auth handler
	if h.LDAPBindFunc(myBindDn, myBindPw) {
		// it worked, result code should be zero
		reth.ResultCode = 0
	}

	return reth.ServeLDAP(ssn, p)

}

// check if this is an unbind
func IsUnbindRequest(p *ber.Packet) bool {
	// message validation
	_, err := ExtractMessageId(p)
	if err != nil {
		ldapdebug("Unable to extract message id: %v", err)
		return false
	}
	err = CheckPacket(p.Children[1], ber.ClassApplication, ber.TypePrimitive, 0x2)
	if err != nil {
		return false
	}
	return true
}

func ExtractMessageId(p *ber.Packet) (int64, error) {

	// check overall packet header
	err := CheckPacket(p, ber.ClassUniversal, ber.TypeConstructed, ber.TagSequence)
	if err != nil {
		return -1, err
	}

	// check type of message id
	err = CheckPacket(p.Children[0], ber.ClassUniversal, ber.TypePrimitive, ber.TagInteger)
	if err != nil {
		return -1, err
	}

	// message id
	messageId := ForceInt64(p.Children[0].Value)

	return messageId, nil
}

func CheckPacket(p *ber.Packet, cl ber.Class, ty ber.Type, ta ber.Tag) error {
	if p.ClassType != cl {
		return fmt.Errorf("Incorrect class, expected %v but got %v", cl, p.ClassType)
	}
	if p.TagType != ty {
		return fmt.Errorf("Incorrect type, expected %v but got %v", cl, p.TagType)
	}
	if p.Tag != ta {
		return fmt.Errorf("Incorrect tag, expected %v but got %v", cl, p.Tag)
	}
	return nil
}

func ForceInt64(v interface{}) int64 {
	switch v := v.(type) {
	case int64:
		return v
	case uint64:
		return int64(v)
	case int32:
		return int64(v)
	case uint32:
		return int64(v)
	case int:
		return int64(v)
	case byte:
		return int64(v)
	default:
		panic(fmt.Sprintf("ForceInt64() doesn't understand values of type: %t", v))
	}
}
