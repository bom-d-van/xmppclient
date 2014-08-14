package xmppclient

import "fmt"

const (
	AffiliationNone    int = 1
	AffiliationOutcast int = 2
	AffiliationMember  int = 3
	AffiliationOwner   int = 4
	AffiliationAdmin   int = 5
	AffiliationInvalid int = 6
	RoleNone           int = 1
	RoleVisitor        int = 2
	RoleParticipant    int = 3
	RoleModerator      int = 4
	RoleInvalid        int = 5
)

func (c *Conn) CreateRoom(jid string) {

}

// <presence from='hag66@shakespeare.lit/pda'
//     to='hoho@conference.shakespeare.lit/nickname'>
//     <x xmlns='http://jabber.org/protocol/muc'/>
// </presence>
func (c *Conn) JoinMUC(jid string, nickname string) error {
	_, err := fmt.Fprintf(
		c.out,
		"<presence from='%s' to='%s/%s'><x xmlns='%s'/></presence>",
		xmlEscape(c.Jid),
		xmlEscape(jid),
		xmlEscape(nickname),
		nsMuc,
	)
	return err
}

//<message
//    from='crone1@shakespeare.lit/desktop'
//    id='nzd143v8'
//    to='coven@chat.shakespeare.lit'>
//  <x xmlns='http://jabber.org/protocol/muc#user'>
//    <invite to='hecate@shakespeare.lit'>
//      <reason>
//        Hey Hecate, this is the place for all good witches!
//      </reason>
//    </invite>
//  </x>
//</message>
func (c *Conn) SendMediatedMucInvitation(to string, roomJid string, reason string) error {
	println(roomJid)
	println(to)
	println(reason)
	s, err := fmt.Fprintf(
		c.out,
		`<message from='%s' to='%s' id='%s'> <x xmlns='%s'><invite to='%s'><reason>%s</reason></invite></x></message>`,
		xmlEscape(c.Jid),
		xmlEscape(roomJid),
		c.getId(),
		nsMucUser,
		xmlEscape(to),
		xmlEscape(reason),
	)
	fmt.Println(s)
	fmt.Println(err)
	return err
}

//<message
//  from='crone1@shakespeare.lit/desktop'
//  to='hecate@shakespeare.lit'>
//    <x xmlns='jabber:x:conference'
//      jid='darkcave@macbeth.shakespeare.lit'
//      reason='Hey Hecate, this is the place for all good witches!'/>
//</message>
func (c *Conn) SendDirectMucInvitation(to string, roomJid string, reason string) error {
	_, err := fmt.Fprintf(
		c.out,
		"<message to='%s' from='%s'><x xmlns='%s' jid='%s' reason='%s'/></message>",
		xmlEscape(to),
		xmlEscape(c.Jid),
		nsConference,
		xmlEscape(roomJid),
		reason,
	)
	return err
}

func (c *Conn) DestroyRoom(jid string) error {
	_, err := fmt.Fprintf(c.out, "<iq from='%s' id='%x' to='%s' type='set'><query xmlns='%s'><destroy jid='%s'></destroy></query></iq>",
		c.Jid, c.getId(), jid, nsMucAdmin, jid)
	return err
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

func (c *Conn) SetRole(roomJid, jid string, role int) error {
	roleName := ""
	switch role {
	case RoleNone:
		roleName = "none"
	case RoleVisitor:
		roleName = "visitor"
	case RoleParticipant:
		roleName = "participant"
	case RoleModerator:
		roleName = "moderator"
	case RoleInvalid:
		roleName = "invalid"
	}
	cookie := c.getId()
	_, err := fmt.Fprintf(
		c.out,
		"<iq from='%s' id='%x' to='%s' type='set'><query xmlns='%s'><item jid='%s' role='%s'/></query></iq>",
		xmlEscape(c.Jid),
		cookie,
		xmlEscape(roomJid),
		nsMucAdmin,
		xmlEscape(jid),
		xmlEscape(roleName),
	)
	return err
}

func (c *Conn) SetAffiliation(roomJid, jid string, affiliation int) error {
	affiliationName := ""
	switch affiliation {
	case AffiliationNone:
		affiliationName = "none"
	case AffiliationOutcast:
		affiliationName = "outcast"
	case AffiliationMember:
		affiliationName = "member"
	case AffiliationOwner:
		affiliationName = "owner"
	case AffiliationAdmin:
		affiliationName = "admin"
	case AffiliationInvalid:
		affiliationName = "invalid"
	}
	cookie := c.getId()
	_, err := fmt.Fprintf(
		c.out,
		"<iq from='%s' id='%x' to='%s' type='set'><query xmlns='%s'><item affiliation='%s' jid='%s'/></query></iq>",
		xmlEscape(c.Jid),
		cookie,
		xmlEscape(roomJid),
		xmlEscape(nsMucAdmin),
		xmlEscape(affiliationName),
		xmlEscape(jid),
	)

	return err
}
