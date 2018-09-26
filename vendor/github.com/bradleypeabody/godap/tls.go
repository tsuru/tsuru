package godap

import (
	"crypto/tls"
	"net"
	"time"
)

func LDAPListenTLS(listenAddr, certFile, keyFile string) (net.Listener, error) {
	config := &tls.Config{}
	// if config.NextProtos == nil {
	// 	config.NextProtos = []string{"http/1.1"}
	// }

	var err error
	config.Certificates = make([]tls.Certificate, 1)
	config.Certificates[0], err = tls.LoadX509KeyPair(certFile, keyFile)
	if err != nil {
		return nil, err
	}

	ln, err := net.Listen("tcp", listenAddr)
	if err != nil {
		return nil, err
	}

	tlsListener := tls.NewListener(ldapTcpKeepAliveListener{ln.(*net.TCPListener)}, config)
	return tlsListener, nil
}

// ldapTcpKeepAliveListener sets TCP keep-alive timeouts on accepted
// connections.
type ldapTcpKeepAliveListener struct {
	*net.TCPListener
}

func (ln ldapTcpKeepAliveListener) Accept() (c net.Conn, err error) {
	tc, err := ln.AcceptTCP()
	if err != nil {
		return
	}
	tc.SetKeepAlive(true)
	tc.SetKeepAlivePeriod(3 * time.Minute)
	return tc, nil
}
