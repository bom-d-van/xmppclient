package xmppclient

import (
	"bytes"
	"crypto/tls"
	"crypto/x509"
	"encoding/base64"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"net"
	"strings"
)

const (
	nsStream  = "http://etherx.jabber.org/streams"
	nsTLS     = "urn:ietf:params:xml:ns:xmpp-tls"
	nsSASL    = "urn:ietf:params:xml:ns:xmpp-sasl"
	nsBind    = "urn:ietf:params:xml:ns:xmpp-bind"
	nsSession = "urn:ietf:params:xml:ns:xmpp-session"
	nsClient  = "jabber:client"
	nsMucUser = "http://jabber.org/protocol/muc#user"
)

// RemoveResourceFromJid returns the user@domain portion of a JID.
func RemoveResourceFromJid(jid string) string {
	slash := strings.Index(jid, "/")
	if slash != -1 {
		return jid[:slash]
	}
	return jid
}

func IsBareJid(jid string) bool {
	if jid == RemoveResourceFromJid(jid) {
		return true
	}
	return false
}

func GetLocalFromJID(jid string) string {
	return strings.Split(jid, "@")[0]
}

func SeparateJidAndResource(fullJid string) (bareJid, resource string) {
	arr := strings.Split(fullJid, "/")
	bareJid = arr[0]
	if len(arr) > 1 {
		resource = arr[1]
	}
	return
}

// Conn represents a connection to an XMPP server.
type Conn struct {
	out    io.Writer
	rawOut io.Writer // doesn't log. Used for <auth>
	in     *xml.Decoder
	Jid    string
	xConn  net.Conn
}

// Stanza represents a message from the XMPP server.
type Stanza struct {
	Name  xml.Name
	Value interface{}
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

// Send an IM message to the given user.
func (c *Conn) SendChatMessage(to, msg string) error {
	return c.sendMessage(to, msg, "chat")
}

// Send an Im message to group chat.
func (c *Conn) SendGroupChatMessage(to, msg string) error {
	return c.sendMessage(to, msg, "groupchat")
}

func (c *Conn) sendMessage(to, msg, chatType string) error {
	_, err := fmt.Fprintf(c.out, "<message to='%s' from='%s' type='%s'><body>%s</body></message>",
		xmlEscape(to), xmlEscape(c.Jid), chatType, xmlEscape(msg))
	return err
}

// Send sends an IM message to the given user.
func (c *Conn) SendComposing(to string) error {
	_, err := fmt.Fprintf(c.out, "<message to='%s' from='%s' type='chat'><composing xmlns='http://jabber.org/protocol/chatstates'/></message>",
		xmlEscape(to), xmlEscape(c.Jid))
	return err
}

// Send sends an IM message to the given user.
func (c *Conn) SendActive(to string) error {
	_, err := fmt.Fprintf(c.out, "<message to='%s' from='%s' type='chat'><active xmlns='http://jabber.org/protocol/chatstates'/></message>",
		xmlEscape(to), xmlEscape(c.Jid))
	return err
}

func (c *Conn) SendDirectMucInvitation(to string, roomJid string, reason string) error {
	_, err := fmt.Fprintf(c.out, "<message to='%s' from='%s'><x xmlns='jabber:x:conference' jid='%s' reason='%s'/></message>",
		xmlEscape(to), xmlEscape(c.Jid), xmlEscape(roomJid), reason)
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
		_, err = fmt.Fprintf(c.out, "<presence to='%s' from='%s'><x xmlns='http://jabber.org/protocol/muc'/></presence>",
			xmlEscape(to), xmlEscape(c.Jid))
	} else {
		_, err = fmt.Fprintf(c.out, "<presence to='%s' from='%s' type='unavailable'/>",
			xmlEscape(to), xmlEscape(c.Jid))
	}

	return
}

func (c *Conn) Close() (err error) {
	return c.xConn.Close()
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

	RequireTLS bool
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

	if config.RequireTLS {
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
	fmt.Println("s")

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

var xmlSpecial = map[byte]string{
	'<':  "&lt;",
	'>':  "&gt;",
	'"':  "&quot;",
	'\'': "&apos;",
	'&':  "&amp;",
}

func xmlEscape(s string) string {
	var b bytes.Buffer
	for i := 0; i < len(s); i++ {
		c := s[i]
		if s, ok := xmlSpecial[c]; ok {
			b.WriteString(s)
		} else {
			b.WriteByte(c)
		}
	}
	return b.String()
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

// RFC 3920  C.1  Streams name space

type streamFeatures struct {
	XMLName    xml.Name `xml:"http://etherx.jabber.org/streams features"`
	StartTLS   tlsStartTLS
	Mechanisms saslMechanisms
	Bind       bindBind
	// This is a hack for now to get around the fact that the new encoding/xml
	// doesn't unmarshal to XMLName elements.
	Session *string `xml:"session"`
}

type streamError struct {
	XMLName xml.Name `xml:"http://etherx.jabber.org/streams error"`
	Any     xml.Name `xml:",any"`
	Text    string   `xml:"text"`
}

// RFC 3920  C.3  TLS name space

type tlsStartTLS struct {
	XMLName  xml.Name `xml:"urn:ietf:params:xml:ns:xmpp-tls starttls"`
	Required xml.Name `xml:"required"`
}

type tlsProceed struct {
	XMLName xml.Name `xml:"urn:ietf:params:xml:ns:xmpp-tls proceed"`
}

type tlsFailure struct {
	XMLName xml.Name `xml:"urn:ietf:params:xml:ns:xmpp-tls failure"`
}

// RFC 3920  C.4  SASL name space

type saslMechanisms struct {
	XMLName   xml.Name `xml:"urn:ietf:params:xml:ns:xmpp-sasl mechanisms"`
	Mechanism []string `xml:"mechanism"`
}

type saslAuth struct {
	XMLName   xml.Name `xml:"urn:ietf:params:xml:ns:xmpp-sasl auth"`
	Mechanism string   `xml:"mechanism,attr"`
}

type saslChallenge string

type saslResponse string

type saslAbort struct {
	XMLName xml.Name `xml:"urn:ietf:params:xml:ns:xmpp-sasl abort"`
}

type saslSuccess struct {
	XMLName xml.Name `xml:"urn:ietf:params:xml:ns:xmpp-sasl success"`
}

type saslFailure struct {
	XMLName xml.Name `xml:"urn:ietf:params:xml:ns:xmpp-sasl failure"`
	Any     xml.Name `xml:",any"`
}

// RFC 3920  C.5  Resource binding name space

type bindBind struct {
	XMLName  xml.Name `xml:"urn:ietf:params:xml:ns:xmpp-bind bind"`
	Resource string   `xml:"resource"`
	Jid      string   `xml:"jid"`
}

// RFC 3921  B.1  jabber:client
type ClientMessage struct {
	XMLName xml.Name `xml:"jabber:client message"`
	From    string   `xml:"from,attr"`
	Id      string   `xml:"id,attr"`
	To      string   `xml:"to,attr"`
	Type    string   `xml:"type,attr"` // chat, error, groupchat, headline, or normal

	// These should technically be []clientText,
	// but string is much more convenient.
	Subject string `xml:"subject"`
	Body    string `xml:"body"`
	Thread  string `xml:"thread"`

	Active      *Active
	Composing   *Composing
	Paused      *Paused
	ConferenceX *ConferenceX `xml:"x"`
}

type Active struct {
	XMLName xml.Name `xml:"http://jabber.org/protocol/chatstates active"`
}

type Composing struct {
	XMLName xml.Name `xml:"http://jabber.org/protocol/chatstates composing"`
}

type Paused struct {
	XMLName xml.Name `xml:"http://jabber.org/protocol/chatstates paused"`
}

type ConferenceX struct {
	XMLName xml.Name `xml:"jabber:x:conference x`
	Jid     string   `xml:"jid,attr"`
	Reason  string   `xml:"reason,attr"`
}

type ClientText struct {
	Lang string `xml:"lang,attr"`
	Body string `xml:",chardata"`
}

func (this *ClientMessage) IsComposing() bool {
	if this.Composing != nil {
		return true
	}
	return false
}

type ClientPresence struct {
	XMLName xml.Name `xml:"jabber:client presence"`
	From    string   `xml:"from,attr"`
	Id      string   `xml:"id,attr"`
	To      string   `xml:"to,attr"`
	Type    string   `xml:"type,attr"` // error, probe, subscribe, subscribed, unavailable, unsubscribe, unsubscribed
	Lang    string   `xml:"lang,attr"`

	Show     string `xml:"show"`   // away, chat, dnd, xa
	Status   string `xml:"status"` // sb []clientText
	Priority string `xml:"priority"`
	C        PresenceC
	X        PresenceX    `xml:"x"`
	Error    *ClientError `xml:"error"`
}

type PresenceC struct {
	XMLName xml.Name `xml:"http://jabber.org/protocol/caps c"`
	Node    string   `xml:"node,attr"`
}

type PresenceX struct {
	XMLName xml.Name        `xml:"jabber:client x`
	Item    MucPresenceItem `xml:"item"`
}

type MucPresenceItem struct {
	Affiliation string `xml:"affiliation,attr"`
	Role        string `xml:"role,attr"`
	Jid         string `xml:"jid,attr"`
}

func (this *ClientPresence) IsMUC() bool {
	if this.X.XMLName.Space == nsMucUser {
		return true
	}
	return false
}

func (this *ClientPresence) IsOnline() bool {
	if this.Type == "" {
		return true
	}
	return false
}

func (this *ClientPresence) IsUnavailable() bool {
	if this.Type == "unavailable" {
		return true
	}
	return false
}

func (this *ClientPresence) HasNode() bool {
	if this.C.Node != "" {
		return true
	}
	return false
}

type ClientIQ struct { // info/query
	XMLName xml.Name    `xml:"jabber:client iq"`
	From    string      `xml:"from,attr"`
	Id      string      `xml:"id,attr"`
	To      string      `xml:"to,attr"`
	Type    string      `xml:"type,attr"` // error, get, result, set
	Error   ClientError `xml:"error"`
	Bind    bindBind    `xml:"bind"`
	Query   []byte      `xml:",innerxml"`
}

type ClientError struct {
	XMLName xml.Name `xml:"jabber:client error"`
	Code    string   `xml:"code,attr"`
	Type    string   `xml:"type,attr"`
	Any     xml.Name `xml:",any"`
	Text    string   `xml:"text"`
}

type Roster struct {
	XMLName xml.Name      `xml:"jabber:iq:roster query"`
	Item    []RosterEntry `xml:"item"`
}

type RosterEntry struct {
	Jid          string   `xml:"jid,attr"`
	Subscription string   `xml:"subscription,attr"`
	Name         string   `xml:"name,attr"`
	Group        []string `xml:"group"`
}

// Scan XML token stream for next element and save into val.
// If val == nil, allocate new element based on proto map.
// Either way, return val.
func next(p *xml.Decoder) (xml.Name, interface{}, error) {
	// Read start element to find out what type we want.
	se, err := nextStart(p)
	if err != nil {
		return xml.Name{}, nil, err
	}

	// Put it in an interface and allocate one.
	var nv interface{}
	switch se.Name.Space + " " + se.Name.Local {
	case nsStream + " features":
		nv = &streamFeatures{}
	case nsStream + " error":
		nv = &streamError{}
	case nsTLS + " starttls":
		nv = &tlsStartTLS{}
	case nsTLS + " proceed":
		nv = &tlsProceed{}
	case nsTLS + " failure":
		nv = &tlsFailure{}
	case nsSASL + " mechanisms":
		nv = &saslMechanisms{}
	case nsSASL + " challenge":
		nv = ""
	case nsSASL + " response":
		nv = ""
	case nsSASL + " abort":
		nv = &saslAbort{}
	case nsSASL + " success":
		nv = &saslSuccess{}
	case nsSASL + " failure":
		nv = &saslFailure{}
	case nsBind + " bind":
		nv = &bindBind{}
	case nsClient + " message":
		nv = &ClientMessage{}
	case nsClient + " presence":
		nv = &ClientPresence{}
	case nsClient + " iq":
		nv = &ClientIQ{}
	case nsClient + " error":
		nv = &ClientError{}
	default:
		return xml.Name{}, nil, errors.New("unexpected XMPP message " +
			se.Name.Space + " <" + se.Name.Local + "/>")
	}

	// Unmarshal into that storage.
	if err = p.DecodeElement(nv, &se); err != nil {
		return xml.Name{}, nil, err
	}
	return se.Name, nv, err
}

type DiscoveryReply struct {
	XMLName    xml.Name `xml:"http://jabber.org/protocol/disco#info query"`
	Identities []DiscoveryIdentity
	Features   []DiscoveryFeature
}

type DiscoveryIdentity struct {
	XMLName  xml.Name `xml:"http://jabber.org/protocol/disco#info identity"`
	Category string   `xml:"category,attr"`
	Type     string   `xml:"type,attr"`
	Name     string   `xml:"name,attr"`
}

type DiscoveryFeature struct {
	XMLName xml.Name `xml:"http://jabber.org/protocol/disco#info feature"`
	Var     string   `xml:"var,attr"`
}

type VersionQuery struct {
	XMLName xml.Name `xml:"jabber:iq:version query"`
}

type VersionReply struct {
	XMLName xml.Name `xml:"jabber:iq:version query"`
	Name    string   `xml:"name"`
	Version string   `xml:"version"`
	OS      string   `xml:"os"`
}

// ErrorReply reflects an XMPP error stanza. See
// http://xmpp.org/rfcs/rfc6120.html#stanzas-error-syntax
type ErrorReply struct {
	XMLName xml.Name    `xml:"error"`
	Type    string      `xml:"type,attr"`
	Error   interface{} `xml:"error"`
}

// ErrorBadRequest reflects a bad-request stanza. See
// http://xmpp.org/rfcs/rfc6120.html#stanzas-error-conditions-bad-request
type ErrorBadRequest struct {
	XMLName xml.Name `xml:"urn:ietf:params:xml:ns:xmpp-stanzas bad-request"`
}

// RosterRequest is used to request that the server update the user's roster.
// See RFC 6121, section 2.3.
type RosterRequest struct {
	XMLName xml.Name          `xml:"jabber:iq:roster query"`
	Item    RosterRequestItem `xml:"item"`
}

type RosterRequestItem struct {
	Jid          string   `xml:"jid,attr"`
	Subscription string   `xml:"subscription,attr"`
	Name         string   `xml:"name,attr"`
	Group        []string `xml:"group"`
}

// An EmptyReply results in in no XML.
type EmptyReply struct {
}
