package keyproofs

import (
	"bufio"
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"math/rand"
	"net/http"
	"net/url"
	"strings"

	"github.com/rs/zerolog/log"
	"github.com/sour-is/keyproofs/pkg/config"
)

type Proof struct {
	Fingerprint string
	Icon        string
	Service     string
	Name        string
	Verify      string
	Link        string
	Status      ProofStatus

	URI *url.URL
}
type Proofs map[string]*Proof

type ProofKey string

func (k ProofKey) Key() interface{} {
	return k
}

type ProofStatus int

const (
	ProofChecking ProofStatus = iota
	ProofError
	ProofInvalid
	ProofVerified
)

func (p ProofStatus) String() string {
	switch p {
	case ProofChecking:
		return "Checking"
	case ProofError:
		return "Error"
	case ProofInvalid:
		return "Invalid"
	case ProofVerified:
		return "Verified"
	default:
		return ""
	}
}

func NewProof(ctx context.Context, uri, fingerprint string) ProofResolver {
	log := log.Ctx(ctx)
	baseURL := config.FromContext(ctx).GetString("base-url")

	p := Proof{Verify: uri, Link: uri, Fingerprint: fingerprint}
	defer log.Info().
		Interface("path", p.URI).
		Str("name", p.Name).
		Str("service", p.Service).
		Str("link", p.Link).
		Msg("Proof")

	var err error

	p.URI, err = url.Parse(uri)
	if err != nil {
		p.Icon = "exclamation-triangle"
		p.Service = "error"
		p.Name = err.Error()

		return &p
	}

	p.Service = p.URI.Scheme

	switch p.URI.Scheme {
	case "dns":
		p.Icon = "fas fa-globe"
		p.Name = p.URI.Opaque
		p.Link = fmt.Sprintf("https://%s", p.URI.Opaque)
		p.Verify = fmt.Sprintf("%s/dns/%s", baseURL, p.URI.Opaque)
		return &httpResolve{p, p.Verify, nil}

	case "xmpp":
		p.Icon = "fas fa-comments"
		p.Name = p.URI.Opaque
		p.Verify = fmt.Sprintf("%s/vcard/%s", baseURL, p.URI.Opaque)
		return &httpResolve{p, p.Verify, nil}

	case "https":
		p.Icon = "fas fa-atlas"
		p.Name = p.URI.Hostname()
		p.Link = fmt.Sprintf("https://%s", p.URI.Hostname())

		switch {
		case strings.HasPrefix(p.URI.Host, "twitter.com"):
			// TODO: Add api authenticated code path.
			if sp := strings.SplitN(p.URI.Path, "/", 3); len(sp) > 1 {
				p.Icon = "fab fa-twitter"
				p.Service = "Twitter"
				p.Name = sp[1]
				p.Link = fmt.Sprintf("https://twitter.com/%s", p.Name)
				p.Verify = fmt.Sprintf("https://twitter.com%s", p.URI.Path)
				url := fmt.Sprintf("https://mobile.twitter.com%s", p.URI.Path)
				return &httpResolve{p, url, nil}
			}

		case strings.HasPrefix(p.URI.Host, "news.ycombinator.com"):
			p.Icon = "fab fa-hacker-news"
			p.Service = "HackerNews"
			p.Name = p.URI.Query().Get("id")
			p.Link = uri
			return &httpResolve{p, p.Verify, nil}

		case strings.HasPrefix(p.URI.Host, "dev.to"):
			if sp := strings.SplitN(p.URI.Path, "/", 3); len(sp) > 1 {
				p.Icon = "fab fa-dev"
				p.Service = "dev.to"
				p.Name = sp[1]
				p.Link = fmt.Sprintf("https://dev.to/%s", p.Name)
				url := fmt.Sprintf("https://dev.to/api/articles/%s/%s", sp[1], sp[2])
				return &httpResolve{p, url, nil}
			}

		case strings.HasPrefix(p.URI.Host, "reddit.com"), strings.HasPrefix(p.URI.Host, "www.reddit.com"):
			var headers map[string]string

			cfg := config.FromContext(ctx)
			if apikey := cfg.GetString("reddit.api-key"); apikey != "" {
				secret := cfg.GetString("reddit.secret")

				headers = map[string]string{
					"Authorization": fmt.Sprintf("basic %s",
						base64.StdEncoding.EncodeToString([]byte(apikey+":"+secret))),
					"User-Agent": "ipseity/0.1.0",
				}
			}

			if sp := strings.SplitN(p.URI.Path, "/", 6); len(sp) > 5 {
				p.Icon = "fab fa-reddit"
				p.Service = "Reddit"
				p.Name = sp[2]
				p.Link = fmt.Sprintf("https://www.reddit.com/user/%s", p.Name)
				url := fmt.Sprintf("https://api.reddit.com/user/%s/comments/%s/%s", sp[2], sp[4], sp[5])
				return &httpResolve{p, url, headers}
			}

		case strings.HasPrefix(p.URI.Host, "gist.github.com"):
			p.Icon = "fab fa-github"
			p.Service = "GitHub"
			if sp := strings.SplitN(p.URI.Path, "/", 3); len(sp) > 2 {
				var headers map[string]string
				if secret := config.FromContext(ctx).GetString("github.secret"); secret != "" {
					headers = map[string]string{
						"Authorization": fmt.Sprintf("bearer %s", secret),
						"User-Agent":    "keyproofs/0.1.0",
					}
				}

				p.Name = sp[1]
				p.Link = fmt.Sprintf("https://github.com/%s", p.Name)
				url := fmt.Sprintf("https://api.github.com/gists/%s", sp[2])
				return &httpResolve{p, url, headers}
			}

		case strings.HasPrefix(p.URI.Host, "lobste.rs"):
			if sp := strings.SplitN(p.URI.Path, "/", 3); len(sp) > 2 {
				p.Icon = "fas fa-list-ul"
				p.Service = "Lobsters"
				p.Name = sp[2]
				p.Link = uri
				p.Verify += ".json"
				return &httpResolve{p, p.Verify, nil}
			}

		case strings.HasSuffix(p.URI.Path, "/gitlab_proof"):
			if sp := strings.SplitN(p.URI.Path, "/", 3); len(sp) > 1 {
				p.Icon = "fab fa-gitlab"
				p.Service = "GetLab"
				p.Name = sp[1]
				p.Link = fmt.Sprintf("https://%s/%s", p.URI.Host, p.Name)
				p.Name = fmt.Sprintf("%s@%s", p.Name, p.URI.Host)
				return &gitlabResolve{p}
			}

		case strings.HasSuffix(p.URI.Path, "/gitea_proof"):
			if sp := strings.SplitN(p.URI.Path, "/", 3); len(sp) > 2 {
				p.Icon = "fas fa-mug-hot"
				p.Service = "Gitea"
				p.Name = sp[1]
				p.Link = fmt.Sprintf("https://%s/%s", p.URI.Host, p.Name)
				p.Name = fmt.Sprintf("%s@%s", p.Name, p.URI.Host)
				url := fmt.Sprintf("https://%s/api/v1/repos/%s/gitea_proof", p.URI.Host, sp[1])
				return &httpResolve{p, url, nil}
			}

		case strings.Contains(p.URI.Path, "/conv/"):
			if sp := strings.SplitN(p.URI.Path, "/", 3); len(sp) == 3 {
				p.Icon = "fas fa-comment-alt"
				p.Service = "Twtxt"
				p.Name = "loading..."
				p.Link = fmt.Sprintf("https://%s", p.URI.Host)

				url := fmt.Sprintf("https://%s/api/v1/conv", p.URI.Host)
				return &twtxtResolve{p, url, sp[2], nil}
			}

		default:
			if sp := strings.SplitN(p.URI.Path, "/", 3); len(sp) > 1 {
				p.Icon = "fas fa-project-diagram"
				p.Service = "Fediverse"
				if len(sp) > 2 && (sp[1] == "u" || sp[1] == "user" || sp[1] == "users") {
					p.Name = sp[2]
				} else {
					p.Name = sp[1]
				}
				p.Name = fmt.Sprintf("%s@%s", p.Name, p.URI.Host)
				p.Link = uri
				return &httpResolve{p, p.Verify, nil}
			}
		}
	default:
		p.Icon = "exclamation-triangle"
		p.Service = "unknown"
		p.Name = "nobody"
	}

	return &p
}

type ProofResolver interface {
	Resolve(context.Context) error
	Proof() *Proof
}

type httpResolve struct {
	proof   Proof
	url     string
	headers map[string]string
}

func (p *httpResolve) Resolve(ctx context.Context) error {
	err := checkHTTP(ctx, p.url, p.proof.Fingerprint, p.headers)
	if err == ErrNoFingerprint {
		p.proof.Status = ProofInvalid
	} else if err != nil {
		p.proof.Status = ProofError
	} else {
		p.proof.Status = ProofVerified
	}
	return err
}
func (p *httpResolve) Proof() *Proof {
	return &p.proof
}

type gitlabResolve struct {
	proof Proof
}

func (r *gitlabResolve) Resolve(ctx context.Context) error {
	uri := r.proof.URI
	r.proof.Status = ProofInvalid

	if sp := strings.SplitN(uri.Path, "/", 3); len(sp) > 1 {
		user := []struct {
			Id int `json:"id"`
		}{}
		if err := httpJSON(ctx, fmt.Sprintf("https://%s/api/v4/users?username=%s", uri.Host, sp[1]), nil, &user); err != nil {
			return err
		}
		if len(user) == 0 {
			return ErrNoFingerprint
		}
		u := user[0]
		url := fmt.Sprintf("https://%s/api/v4/users/%d/projects", uri.Host, u.Id)
		proofs := []struct {
			Description string
		}{}
		if err := httpJSON(ctx, url, nil, &proofs); err != nil {
			return err
		}
		if len(proofs) == 0 {
			return ErrNoFingerprint
		}
		ck := fmt.Sprintf("[Verifying my OpenPGP key: openpgp4fpr:%s]", strings.ToLower(r.proof.Fingerprint))
		for _, p := range proofs {
			if strings.Contains(p.Description, ck) {
				r.proof.Status = ProofVerified
				return nil
			}
		}
	}

	return ErrNoFingerprint
}
func (r *gitlabResolve) Proof() *Proof {
	return &r.proof
}

func (p *Proof) Resolve(ctx context.Context) error {
	return fmt.Errorf("Not Implemented")
}
func (p *Proof) Proof() *Proof {
	return p
}

type twtxtResolve struct {
	proof   Proof             `json:"-"`
	url     string            `json:"-"`
	Hash    string            `json:"hash"`
	headers map[string]string `json:"-"`
}

func (t *twtxtResolve) Resolve(ctx context.Context) error {
	t.proof.Status = ProofInvalid

	twt := struct {
		Twts []struct {
			Text  string `json:"text"`
			Twter struct{ Nick string }
		} `json:"twts"`
	}{}

	if err := postJSON(ctx, t.url, nil, t, &twt); err != nil {
		return err
	}
	if len(twt.Twts) > 0 {
		t.proof.Name = twt.Twts[0].Twter.Nick
		t.proof.Link += "/user/" + twt.Twts[0].Twter.Nick

		ck := fmt.Sprintf("[Verifying my OpenPGP key: openpgp4fpr:%s]", strings.ToLower(t.proof.Fingerprint))
		if strings.Contains(twt.Twts[0].Text, ck) {
			t.proof.Status = ProofVerified
			return nil
		}
	}

	return ErrNoFingerprint
}
func (t *twtxtResolve) Proof() *Proof {
	return &t.proof
}

func checkHTTP(ctx context.Context, uri, fingerprint string, hdr map[string]string) error {
	log := log.Ctx(ctx)

	log.Info().
		Str("URI", uri).
		Str("fp", fingerprint).
		Msg("Proof")

	req, err := http.NewRequestWithContext(ctx, "GET", uri, nil)
	if err != nil {
		log.Err(err)
		return err
	}
	req.Header.Set("Accept", "application/json")
	for k, v := range hdr {
		req.Header.Set(k, v)
	}

	res, err := http.DefaultClient.Do(req)
	if err != nil {
		log.Err(err)
		return err
	}
	defer res.Body.Close()

	ts := rand.Int63()
	log.Info().Str("uri", uri).Int64("ts", ts).Msg("Reading data")
	defer log.Info().Str("uri", uri).Int64("ts", ts).Msg("Read data")

	scan := bufio.NewScanner(res.Body)
	for scan.Scan() {
		if strings.Contains(strings.ToUpper(scan.Text()), fingerprint) {
			return nil
		}
	}

	return ErrNoFingerprint
}

var ErrNoFingerprint = errors.New("fingerprint not found")

func httpJSON(ctx context.Context, uri string, hdr map[string]string, dst interface{}) error {
	log := log.Ctx(ctx)

	log.Info().Str("URI", uri).Msg("httpJSON")

	req, err := http.NewRequestWithContext(ctx, "GET", uri, nil)
	if err != nil {
		log.Err(err)
		return err
	}
	req.Header.Set("Accept", "application/json")
	for k, v := range hdr {
		req.Header.Set(k, v)
	}

	res, err := http.DefaultClient.Do(req)
	if err != nil {
		log.Err(err)
		return err
	}
	defer res.Body.Close()

	return json.NewDecoder(res.Body).Decode(dst)
}

func postJSON(ctx context.Context, uri string, hdr map[string]string, payload, dst interface{}) error {
	log := log.Ctx(ctx)

	log.Info().Str("URI", uri).Msg("postJSON")

	body, err := json.Marshal(payload)
	if err != nil {
		log.Err(err).Send()
		return err
	}
	buf := bytes.NewBuffer(body)

	req, err := http.NewRequestWithContext(ctx, "POST", uri, buf)
	if err != nil {
		log.Err(err).Send()
		return err
	}

	req.Header.Set("Accept", "application/json")
	for k, v := range hdr {
		req.Header.Set(k, v)
	}

	res, err := http.DefaultClient.Do(req)
	if err != nil {
		log.Err(err)
		return err
	}
	defer res.Body.Close()

	return json.NewDecoder(res.Body).Decode(dst)
}
