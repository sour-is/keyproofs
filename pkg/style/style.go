package style

import (
	"context"
	"crypto/md5"
	"fmt"
	"net"
	"strings"

	"github.com/lucasb-eyer/go-colorful"
	"github.com/rs/zerolog/log"
)

var pixl = "data:image/png;base64,iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mNkYAAAAAYAAjCB0C8AAAAASUVORK5CYII="

type Key string

func (s Key) Key() interface{} {
	return s
}

type Style struct {
	Avatar,
	Cover,
	Background string

	Palette []string
}

func GetStyle(ctx context.Context, email string) (*Style, error) {
	log := log.Ctx(ctx)

	avatarHost, styleHost, err := GetSRV(ctx, email)
	if err != nil {
		return nil, err
	}
	log.Info().Str("avatar", avatarHost).Str("style", styleHost).Msg("getStyle")

	hash := md5.New()
	email = strings.TrimSpace(strings.ToLower(email))
	_, _ = hash.Write([]byte(email))
	id := hash.Sum(nil)

	style := &Style{}

	style.Palette = GetPalette(fmt.Sprintf("#%x", id[:3]))
	style.Avatar = fmt.Sprintf("https://%s/avatar/%x", avatarHost, id)
	style.Cover = pixl
	style.Background = pixl

	if styleHost != "" {
		style.Cover = fmt.Sprintf("https://%s/cover/%x", styleHost, id)
		style.Background = fmt.Sprintf("https://%s/bg/%x", styleHost, id)
	}

	return style, err
}

func GetSRV(ctx context.Context, email string) (avatar string, style string, err error) {

	// Defaults
	style = ""
	avatar = "www.libravatar.org"

	parts := strings.SplitN(email, "@", 2)
	if _, srv, err := net.DefaultResolver.LookupSRV(ctx, "style-sec", "tcp", parts[1]); err == nil {
		if len(srv) > 0 {
			style = strings.TrimSuffix(srv[0].Target, ".")
			avatar = strings.TrimSuffix(srv[0].Target, ".")

			return avatar, style, err
		}
	}

	if _, srv, err := net.DefaultResolver.LookupSRV(ctx, "avatars-sec", "tcp", parts[1]); err == nil {
		if len(srv) > 0 {
			avatar = strings.TrimSuffix(srv[0].Target, ".")

			return avatar, style, err
		}
	}

	return
}

// getPalette maes a complementary color palette. https://play.golang.org/p/nBXLUocGsU5
func GetPalette(hex string) []string {
	reference, _ := colorful.Hex(hex)
	reference = sat(lum(reference, 0, .5), 0, .5)

	white := colorful.Color{R: 1, G: 1, B: 1}
	black := colorful.Color{R: 0, G: 0, B: 0}
	accentA := hue(reference, 60)
	accentB := hue(reference, -60)
	accentC := hue(reference, -180)

	return append(
		[]string{},

		white.Hex(),
		lum(reference, .4, .6).Hex(),
		reference.Hex(),
		lum(reference, .4, 0).Hex(),
		black.Hex(),

		lum(accentA, .4, .6).Hex(),
		accentA.Hex(),
		lum(accentA, .4, 0).Hex(),

		lum(accentB, .4, .6).Hex(),
		accentB.Hex(),
		lum(accentB, .4, 0).Hex(),

		lum(accentC, .4, .6).Hex(),
		accentC.Hex(),
		lum(accentC, .4, 0).Hex(),
	)
}
func hue(in colorful.Color, H float64) colorful.Color {
	h, s, l := in.Hsl()
	return colorful.Hsl(h+H, s, l)
}
func sat(in colorful.Color, S, V float64) colorful.Color {
	h, s, l := in.Hsl()
	return colorful.Hsl(h, V+s*S, l)
}
func lum(in colorful.Color, L, V float64) colorful.Color {
	h, s, l := in.Hsl()
	return colorful.Hsl(h, s, V+l*L)
}
