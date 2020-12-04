package app_vcard

import (
	"encoding/xml"

	"gosrc.io/xmpp/stanza"
)

type VCard struct {
	XMLName     xml.Name `xml:"vcard-temp vCard"`
	FullName    string   `xml:"FN"`
	NickName    string   `xml:"NICKNAME"`
	Description string   `xml:"DESC"`
	URL         string   `xml:"URL"`
}

func NewVCard() *VCard {
	return &VCard{}
}

func (c *VCard) Namespace() string {
	return c.XMLName.Space
}

func (c *VCard) GetSet() *stanza.ResultSet {
	return nil
}

func (c *VCard) String() string {
	b, _ := xml.MarshalIndent(c, "", "  ")
	return string(b)
}

func init() {
	stanza.TypeRegistry.MapExtension(stanza.PKTIQ, xml.Name{Space: "vcard-temp", Local: "vCard"}, VCard{})
}
