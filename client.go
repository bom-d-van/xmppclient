package xmppclient

import (
	"crypto/tls"
	"crypto/x509"
	"encoding/base64"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"net"
)

// Conn represents a connection to an XMPP server.
type Conn struct {
	out    io.Writer
	rawOut io.Writer // doesn't log. Used for <auth>
	in     *xml.Decoder
	xConn  net.Conn

	Jid           string
	Domain        string
	password      string
	escapedDomain string
	escapedJid    string

	OnlineRoster []string
	Handler      Handler
}

// Config contains options for an XMPP connection.
type Config struct {
	// InLog is an optional Writer which receives the raw contents of the
	// XML from the server.
	InLog io.Writer
	// OutLog is an option Writer which receives the raw XML sent to the
	// server.
	OutLog io.Writer
	// Log is an optional Writer which receives human readable log messages
	// during the connection.
	Log io.Writer

	TLSRequired bool
}

// Dial creates a new connection to an XMPP server and authenticates as the
// given user.
func Dial(address, user, domain, password string, config *Config) (c *Conn, err error) {
	c = new(Conn)

	var log io.Writer
	if config != nil && config.Log != nil {
		log = config.Log
	}

	if log != nil {
		io.WriteString(log, "Making TCP connection to "+address+"\n")
	}

	if c.xConn, err = net.Dial("tcp", address); err != nil {
		return nil, err
	}

	if config.TLSRequired {
		tlsConn, err := startTLSNegotiation(c, domain, log, config)
		if err != nil {
			return nil, err
		}
		c.in, c.out = makeInOut(tlsConn, config)
		c.rawOut = tlsConn
	} else {
		c.in, c.out = makeInOut(c.xConn, config)
		c.rawOut = c.xConn
	}

	var features streamFeatures
	if features, err = c.getFeatures(domain); err != nil {
		return nil, err
	}

	if log != nil {
		io.WriteString(log, "Authenticating as "+user+"\n")
	}
	if err := c.authenticate(features, user, password); err != nil {
		return nil, err
	}

	if log != nil {
		io.WriteString(log, "Authentication successful\n")
	}

	if features, err = c.getFeatures(domain); err != nil {
		return nil, err
	}

	// Send IQ message asking to bind to the local user name.
	fmt.Fprintf(c.out, "<iq type='set' id='bind_1'><bind xmlns='%s'/></iq>", nsBind)
	var iq ClientIQ
	if err = c.in.DecodeElement(&iq, nil); err != nil {
		return nil, errors.New("unmarshal <iq>: " + err.Error())
	}
	if &iq.Bind == nil {
		return nil, errors.New("<iq> result missing <bind>")
	}
	c.Jid = iq.Bind.Jid // our local id
	if log != nil {
		io.WriteString(log, c.Jid+"\n")
	}

	c.password = password
	c.Domain = domain
	c.escapedJid = xmlEscape(c.Jid)
	c.escapedDomain = xmlEscape(c.Domain)

	if features.Session != nil {
		// The server needs a session to be established. See RFC 3921,
		// section 3.
		fmt.Fprintf(c.out, "<iq to='%s' type='set' id='sess_1'><session xmlns='%s'/></iq>", domain, nsSession)
		if err = c.in.DecodeElement(&iq, nil); err != nil {
			return nil, errors.New("xmpp: unmarshal <iq>: " + err.Error())
		}
		if iq.Type != "result" {
			return nil, errors.New("xmpp: session establishment failed")
		}
	}

	return c, nil
}

func startTLSNegotiation(c *Conn, domain string, log io.Writer, config *Config) (conn io.ReadWriter, err error) {
	c.in, c.out = makeInOut(c.xConn, config)

	features, err := c.getFeatures(domain)
	if err != nil {
		return
	}

	if features.StartTLS.XMLName.Local == "" {
		err = errors.New("xmpp: server doesn't support TLS")
		return
	}

	fmt.Fprintf(c.out, "<starttls xmlns='%s'/>", nsTLS)

	proceed, err := nextStart(c.in)
	if err != nil {
		return
	}
	if proceed.Name.Space != nsTLS || proceed.Name.Local != "proceed" {
		err = errors.New("xmpp: expected <proceed> after <starttls> but got <" + proceed.Name.Local + "> in " + proceed.Name.Space)
		return
	}

	if log != nil {
		io.WriteString(log, "Starting TLS handshake\n")
	}

	tlsConn := tls.Client(c.xConn, nil)
	if err = tlsConn.Handshake(); err != nil {
		return
	}

	tlsState := tlsConn.ConnectionState()
	if len(tlsState.VerifiedChains) == 0 {
		err = errors.New("xmpp: failed to verify TLS certificate")
		return
	}

	if log != nil {
		for i, cert := range tlsState.VerifiedChains[0] {
			fmt.Fprintf(log, "  certificate %d: %s\n", i, certName(cert))
		}
	}

	if err = tlsConn.VerifyHostname(domain); err != nil {
		err = errors.New("xmpp: failed to match TLS certificate to name: " + err.Error())
		return
	}

	// c.in, c.out = makeInOut(tlsConn, config)
	// c.rawOut = tlsConn
	conn = tlsConn
	return
}

func makeInOut(conn io.ReadWriter, config *Config) (in *xml.Decoder, out io.Writer) {
	if config != nil && config.InLog != nil {
		in = xml.NewDecoder(io.TeeReader(conn, config.InLog))
	} else {
		in = xml.NewDecoder(conn)
	}

	if config != nil && config.OutLog != nil {
		out = io.MultiWriter(conn, config.OutLog)
	} else {
		out = conn
	}

	return
}

func (c *Conn) authenticate(features streamFeatures, user, password string) (err error) {
	havePlain := false
	for _, m := range features.Mechanisms.Mechanism {
		if m == "PLAIN" {
			havePlain = true
			break
		}
	}
	if !havePlain {
		return errors.New("xmpp: PLAIN authentication is not an option")
	}

	// Plain authentication: send base64-encoded \x00 user \x00 password.
	raw := "\x00" + user + "\x00" + password
	enc := make([]byte, base64.StdEncoding.EncodedLen(len(raw)))
	base64.StdEncoding.Encode(enc, []byte(raw))
	fmt.Fprintf(c.rawOut, "<auth xmlns='%s' mechanism='PLAIN'>%s</auth>\n", nsSASL, enc)

	// Next message should be either success or failure.
	name, val, err := next(c.in)
	switch v := val.(type) {
	case *saslSuccess:
	case *saslFailure:
		// v.Any is type of sub-element in failure,
		// which gives a description of what failed.
		return errors.New("xmpp: authentication failure: " + v.Any.Local)
	default:
		return errors.New("expected <success> or <failure>, got <" + name.Local + "> in " + name.Space)
	}

	return nil
}

func certName(cert *x509.Certificate) string {
	name := cert.Subject
	ret := ""

	for _, org := range name.Organization {
		ret += "O=" + org + "/"
	}
	for _, ou := range name.OrganizationalUnit {
		ret += "OU=" + ou + "/"
	}
	if len(name.CommonName) > 0 {
		ret += "CN=" + name.CommonName + "/"
	}
	return ret
}

// rfc3920 section 5.2
func (c *Conn) getFeatures(domain string) (features streamFeatures, err error) {
	if _, err = fmt.Fprintf(c.out, "<?xml version='1.0'?><stream:stream to='%s' xmlns='%s' xmlns:stream='%s' version='1.0'>\n", xmlEscape(domain), nsClient, nsStream); err != nil {
		return
	}

	se, err := nextStart(c.in)
	if err != nil {
		return
	}
	if se.Name.Space != nsStream || se.Name.Local != "stream" {
		err = errors.New("xmpp: expected <stream> but got <" + se.Name.Local + "> in " + se.Name.Space)
		return
	}

	// Now we're in the stream and can use Unmarshal.
	// Next message should be <features> to tell us authentication options.
	// See section 4.6 in RFC 3920.
	if err = c.in.DecodeElement(&features, nil); err != nil {
		err = errors.New("unmarshal <features>: " + err.Error())
		return
	}

	return
}

// Scan XML token stream to find next StartElement.
func nextStart(p *xml.Decoder) (elem xml.StartElement, err error) {
	for {
		var t xml.Token
		t, err = p.Token()
		if err != nil {
			return
		}
		switch t := t.(type) {
		case xml.StartElement:
			elem = t
			return
		}
	}
	panic("unreachable")
}

// Next reads stanzas from the server. If the stanza is a reply, it dispatches
// it to the correct channel and reads the next message. Otherwise it returns
// the stanza for processing.
func (c *Conn) Next(ch chan<- Stanza) {
	stanza, err := new(Stanza), (error)(nil)
	for {
		if stanza.Name, stanza.Value, err = next(c.in); err != nil {
			return
		}

		ch <- *stanza
	}
}

func (c *Conn) Listen() {
	for {
		stanza, err := new(Stanza), (error)(nil)
		if stanza.Name, stanza.Value, err = next(c.in); err != nil {
			return
		}
		switch stanza.Name.Local {
		case "presence":
			presence := stanza.Value.(*ClientPresence)
			c.OnlineRoster = append(c.OnlineRoster, presence.From)
			if c.Handler != nil {
				c.Handler.RecvPresence(presence)
			}
		case "message":
			if c.Handler != nil {
				c.Handler.RecvMsg(stanza.Value.(*ClientMessage))
			}
		}
	}
}

// Send an IM message to the given user.
func (c *Conn) Send(to, msg string) error {
	return c.sendMessage(to, msg, "chat")
}

// Send an Im message to group chat.
func (c *Conn) SendGroupChatMessage(to, msg string) error {
	return c.sendMessage(to, msg, "groupchat")
}

func (c *Conn) sendMessage(to, msg, chatType string) error {
	_, err := fmt.Fprintf(
		c.out,
		"<message to='%s' from='%s' type='%s'><body>%s</body></message>",
		xmlEscape(to),
		xmlEscape(c.Jid),
		chatType,
		xmlEscape(msg),
	)
	return err
}

// Send sends an IM message to the given user.
func (c *Conn) SendComposing(to string) error {
	_, err := fmt.Fprintf(
		c.out,
		"<message to='%s' from='%s' type='chat'><composing xmlns='http://jabber.org/protocol/chatstates'/></message>",
		xmlEscape(to),
		xmlEscape(c.Jid),
	)
	return err
}

// Send sends an IM message to the given user.
func (c *Conn) SendActive(to string) error {
	_, err := fmt.Fprintf(
		c.out,
		"<message to='%s' from='%s' type='chat'><active xmlns='http://jabber.org/protocol/chatstates'/></message>",
		xmlEscape(to),
		xmlEscape(c.Jid),
	)
	return err
}

func (c *Conn) SendDirectMucInvitation(to string, roomJid string, reason string) error {
	_, err := fmt.Fprintf(
		c.out,
		"<message to='%s' from='%s'><x xmlns='jabber:x:conference' jid='%s' reason='%s'/></message>",
		xmlEscape(to),
		xmlEscape(c.Jid),
		xmlEscape(roomJid),
		reason,
	)
	return err
}

func (c *Conn) SignalPresence(state string) error {
	_, err := fmt.Fprintf(c.out, "<presence><show>%s</show></presence>", xmlEscape(state))
	return err
}

func (c *Conn) SliencePresence() error {
	_, err := fmt.Fprintf(c.out, "<presence><priority>-1</priority></presence>")
	return err
}

func (c *Conn) PresenceMuc(to string, isJoined bool) (err error) {
	if isJoined {
		goto joined
	}
	_, err = fmt.Fprintf(
		c.out,
		"<presence to='%s' from='%s' type='unavailable'/>",
		xmlEscape(to),
		xmlEscape(c.Jid),
	)
	return

joined:
	_, err = fmt.Fprintf(
		c.out,
		"<presence to='%s' from='%s'><x xmlns='http://jabber.org/protocol/muc'/></presence>",
		xmlEscape(to),
		xmlEscape(c.Jid),
	)
	return
}

// <iq from='hag66@shakespeare.lit/pda'
//     id='zb8q41f4'
//     to='chat.shakespeare.lit'
//     type='get'>
//   <query xmlns='http://jabber.org/protocol/disco#items'/>
// </iq>
func (c *Conn) DiscoverRooms() {
	fmt.Fprintf(
		c.out,
		"<iq type='get' from='%s' to='conference.localhost'><query xmlns='http://jabber.org/protocol/disco#items'/></iq>",
		c.escapedJid,
		// c.escapedDomain,
	)

	for {
		// token, err2 := c.in.Token()
		// fmt.Printf("%+v\n", token)
		// fmt.Printf("%+v\n", err2)

		iq := ClientIQ{}
		if err := c.in.DecodeElement(&iq, nil); err != nil {
			fmt.Printf("error %+v\n", err)
		}
		fmt.Printf("Query: %+v\n", string(iq.Query))
	}
}

func (c *Conn) Close() (err error) {
	return c.xConn.Close()
}
