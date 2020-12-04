package entity

import (
	"fmt"
	"io"
	"net/mail"

	"github.com/sour-is/crypto/openpgp"
	"github.com/sour-is/crypto/openpgp/packet"
)

type Key string

func (k Key) Key() interface{} {
	return k
}

type Entity struct {
	Primary       *mail.Address
	SelfSignature *packet.Signature
	Emails        []*mail.Address
	Fingerprint   string
	Proofs        []string
	ArmorText     string
	entity        *openpgp.Entity
}

func (e *Entity) Serialize(f io.Writer) error {
	return e.entity.Serialize(f)
}

func GetOne(lis openpgp.EntityList) (*Entity, error) {
	entity := &Entity{}
	var err error

	for _, e := range lis {
		if e == nil {
			continue
		}
		if e.PrimaryKey == nil {
			continue
		}

		entity.entity = e
		entity.Fingerprint = fmt.Sprintf("%X", e.PrimaryKey.Fingerprint)

		for name, ident := range e.Identities {
			// Pick first identity
			if entity.Primary == nil {
				entity.Primary, err = mail.ParseAddress(name)
				if err != nil {
					return entity, err
				}
			}
			// If one is marked primary use that
			if ident.SelfSignature != nil && ident.SelfSignature.IsPrimaryId != nil && *ident.SelfSignature.IsPrimaryId {
				entity.Primary, err = mail.ParseAddress(name)
				if err != nil {
					return entity, err
				}

			} else {
				var email *mail.Address
				if email, err = mail.ParseAddress(name); err != nil {
					return entity, err
				}
				if email.Address != entity.Primary.Address {
					entity.Emails = append(entity.Emails, email)
				}
			}

			// If identity is self signed read notation data.
			if ident.SelfSignature != nil && ident.SelfSignature.NotationData != nil {
				entity.SelfSignature = ident.SelfSignature
				// Get proofs and append to list.
				if proofs, ok := ident.SelfSignature.NotationData["proof@metacode.biz"]; ok {
					entity.Proofs = append(entity.Proofs, proofs...)
				}
			}
		}
		break
	}

	if entity.Primary == nil {
		entity.Primary, _ = mail.ParseAddress("nobody@nodomain.xyz")
	}

	return entity, err
}
