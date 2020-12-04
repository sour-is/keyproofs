package app_vcard

import (
	"context"
	"encoding/xml"
	"fmt"

	"github.com/rs/zerolog/log"
	"github.com/sour-is/keyproofs/pkg/graceful"
	"gosrc.io/xmpp"
	"gosrc.io/xmpp/stanza"
)

type connection struct {
	client xmpp.StreamClient
}

func NewXMPP(ctx context.Context, config *xmpp.Config) (*connection, error) {
	log := log.Ctx(ctx)
	wg := graceful.WaitGroup(ctx)

	router := xmpp.NewRouter()
	conn := &connection{}

	cl, err := xmpp.NewClient(config, router, func(err error) { log.Error().Err(err).Send() })
	if err != nil {
		return nil, err
	}
	conn.client = cl

	sc := xmpp.NewStreamManager(cl, func(c xmpp.Sender) { log.Info().Msg("XMPP Client connected.") })

	wg.Go(func() error {
		log.Debug().Msg("starting XMPP")
		return sc.Run()
	})

	go func() {
		<-ctx.Done()
		sc.Stop()
		log.Info().Msg("XMPP Client shutdown.")
	}()

	return conn, err
}

func (conn *connection) GetXMPPVCard(ctx context.Context, jid string) (vc *VCard, err error) {
	log := log.Ctx(ctx)

	var iq *stanza.IQ
	iq, err = stanza.NewIQ(stanza.Attrs{To: jid, Type: "get"})
	if err != nil {
		return nil, err
	}
	iq.Payload = NewVCard()

	var ch chan stanza.IQ
	ch, err = conn.client.SendIQ(ctx, iq)
	if err != nil {
		return nil, err
	}

	select {
	case result := <-ch:
		b, _ := xml.MarshalIndent(result, "", "  ")
		log.Debug().Msgf("%s", b)
		if vcard, ok := result.Payload.(*VCard); ok {
			return vcard, nil
		}
		return nil, fmt.Errorf("bad response: %s", result.Payload)

	case <-ctx.Done():
	}

	return nil, fmt.Errorf("timeout requesting vcard for %s", jid)
}
