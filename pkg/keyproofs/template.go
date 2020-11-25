package keyproofs

type page struct {
	AppName string
	Entity  *Entity
	Style   *Style
	Proofs  *Proofs

	Markdown   string
	HasProofs  bool
	IsComplete bool
	Err        error
}

var pageTPL = `
<html>
<head>
	{{if not .IsComplete}}<meta http-equiv="refresh" content="1">{{end}}

	<link href="https://pagecdn.io/lib/bootstrap/4.5.1/css/bootstrap.min.css"       rel="stylesheet" crossorigin="anonymous" integrity="sha256-VoFZSlmyTXsegReQCNmbXrS4hBBUl/cexZvPmPWoJsY=" >
	<link href="https://pagecdn.io/lib/font-awesome/5.14.0/css/fontawesome.min.css" rel="stylesheet" crossorigin="anonymous" integrity="sha256-7YMlwkILTJEm0TSengNDszUuNSeZu4KTN3z7XrhUQvc=" >
	<link href="https://pagecdn.io/lib/font-awesome/5.14.0/css/solid.min.css"       rel="stylesheet" crossorigin="anonymous" integrity="sha256-s0DhrAmIsT5gZ3X4f+9wIXUbH52CMiqFAwgqCmdPoec=" >
	<link href="https://pagecdn.io/lib/font-awesome/5.14.0/css/regular.min.css"     rel="stylesheet" crossorigin="anonymous" integrity="sha256-FAKIbnpfWhK6v5Re+NAi9n+5+dXanJvXVFohtH6WAuw=" >
	<link href="https://pagecdn.io/lib/font-awesome/5.14.0/css/brands.min.css"      rel="stylesheet" crossorigin="anonymous" integrity="sha256-xN44ju35FR+kTO/TP/UkqrVbM3LpqUI1VJCWDGbG1ew=" >

{{ with .Style }}
	<style>
		@font-face { font-family: "Font Awesome 5 Free"; font-weight: 900; src: url(https://pagecdn.io/lib/font-awesome/5.14.0/webfonts/fa-solid-900.woff2); }
		@font-face { font-family: "Font Awesome 5 Free"; font-weight: 400; src: url(https://pagecdn.io/lib/font-awesome/5.14.0/webfonts/fa-regular-400.woff2); }
		@font-face { font-family: "Font Awesome 5 Brands"; src: url(https://pagecdn.io/lib/font-awesome/5.14.0/webfonts/fa-brands-400.woff2); }

		{{range $i, $val := .Palette}}.fg-color-{{$i}} { color: {{$val}}; }
		{{end}}

		{{range $i, $val := .Palette}}.bg-color-{{$i}} { background-color: {{$val}}; }
		{{end}}

		body {
			background-image: url('{{.Background}}');
			background-repeat: repeat;
			background-color: {{index .Palette 7}};
			padding-top: 1em;
		}
		.heading {
			background-image: url('{{.Cover}}');
			background-size: cover;
			background-repeat: no-repeat;
			background-color: {{index .Palette 3}};
		}
		.shade { background-color: {{index .Palette 3}}80; border-radius: .25rem;}
		.lead { padding:0; margin:0;  }
		.scroll { height: 20em; overflow: scroll; }
		@media only screen and (max-width: 991px) {
			.jumbotron h1 { font-size: 2rem; }
			.jumbotron .lead { font-size: 1.0rem; }
		}

		@media only screen and (max-width: 768px) {
			.center-xs { text-align: center; width: 100% }
			.center-sm { text-align: center; width: 100% }
			.center-md { text-align: center; width: 100% }
			.jumbotron h1 { font-size: 2rem; }
			.jumbotron .lead { font-size: 1.0rem; }
		}

		 @media only screen and (max-width: 576px) {
			.center-xs { text-align: center; width: 100% }
			.center-sm { text-align: center; width: 100% }
			.center-md { text-align: center; width: 100% }
			.jumbotron .lead { font-size: 0.8rem; }
			body { font-size: 0.8rem; }
		}
	</style>
{{end}}
</head>

<body>
	<div class="container">
		<div class="card">
			{{template "content" .}}

			<div class="card-footer text-muted text-center">
				<a href="/">{{.AppName}}</a>
				| &copy; 2020 Sour.is
				| <a href="/id/me@sour.is">About me</a>
				| <a href="https://github.com/sour-is/keyproofs">GitHub</a>
				| Inspired by <a href="https://keyoxide.org/">keyoxide</a>
			</div>
		</div>
	</div>
</body>
</html>
`

var homeTPL = `
{{define "content"}}
<div class="jumbotron heading">
	<div class="container">
		<div class="row shade">
			<div class="col-md">
				<h1 class="display-8 fg-color-8">Key Proofs</h1>
				<p class="lead fg-color-11">Verify social identitys using OpenPGP</p>
			</div>
		</div>
	</div>
</div>
<br/>
<div class="card">
	<div class="card-body">
		<form method="GET" action="/">
			<div class="input-group mb-3">
				<input type="text"
					   name="id"
					   class="form-control"
					   placeholder="Email or Fingerprint..."
					   aria-label="Email or Fingerprint"
					   aria-describedby="button-addon" />
				<div class="input-group-append">
					<button class="btn btn-outline-secondary" type="submit" id="button-addon">GO</button>
				</div>
			</div>
		</form>
	</div>
</div>
<div class="container"> {{.Markdown | markDown}} </div>
{{end}}
`

var proofTPL = `
{{define "content"}}
<div class="jumbotron heading">
	<div class="container">
		<div class="row shade">
		{{ with .Err }}
			<div class="col-xs center-md">
				<i class="fas fa-exclamation-triangle fa-4x fg-color-11"></i>
			</div>

			<div class="col-md">
				<h1 class="display-8 fg-color-8">Something went wrong...</h1>
				<pre class="fg-color-11">{{.}}</pre>
			</div>
		{{else}}
			{{ with .Style }}
				<div class="col-xs center-md">
					<img src="{{.Avatar}}" class="img-thumbnail" alt="avatar" style="width:88px; height:88px">
				</div>
			{{end}}

			{{with .Entity}}
				<div class="col-md center-md">
					<h1 class="display-8 fg-color-8">{{.Primary.Name}}</h1>
					<p class="lead fg-color-11"><i class="fas fa-fingerprint"></i> {{.Fingerprint}}</p>
				</div>
				<div class="col-xs center-md">
					<img src="/qr?s=-2&c=OPENPGP4FPR%3A{{.Fingerprint}}" class="img-thumbnail" alt="qrcode" style="width:88px; height:88px">
				</div>
			{{else}}
				<div class="col-md">
					<h1 class="display-8 fg-color-8">Loading...</h1>
					<p class="lead fg-color-11">Reading key from remote service.</p>
				</div>
			{{end}}
		{{end}}
		</div>
	</div>
</div>

<div class="container">
	<div class="row">
		<div class="col-lg-4 col-md-12 col-sm-12 col-xs-12">
		{{ with .Entity }}
			<div class="card">
				<div class="card-header">Contact</div>
				<div class="list-group list-group-flush">
					{{with .Primary}}<a href="mailto:{{.Address}}" class="list-group-item list-group-item-action"><i class="fas fa-envelope"></i> <b>{{.Name}} &lt;{{.Address}}&gt;</b> <span class="badge badge-secondary">Primary</span></a>{{end}}
					{{range .Emails}}<a href="mailto:{{.Address}}" class="list-group-item list-group-item-action"><i class="far fa-envelope"></i> {{.Name}} &lt;{{.Address}}&gt;</a>{{end}}
				</div>
			</div>
			<br />
		{{end}}

		{{if .HasProofs}}
		{{with .Proofs}}
			<div class="card">
				<div class="card-header">Proofs</div>
					<ul class="list-group list-group-flush">
						{{range .}}
						<li class="list-group-item">
							<div>
								<a title="{{.Link}}" class="font-weight-bold" href="{{.Link}}">
									<i title="{{.Service}}" class="{{.Icon}}"></i>
									{{.Name}}
								</a>

								{{if eq .Status 0}}
									<a class="text-muted" href="{{.Verify}}"> <i class="fas fa-ellipsis-h"> Checking</i></a>
								{{else if eq .Status 1}}
									<a class="text-warning" href="{{.Verify}}"> <i class="fas fa-exclamation-triangle"></i> Error</a>
								{{else if eq .Status 2}}
									<a class="text-danger" href="{{.Verify}}"> <i class="far fa-times-circle"></i> Invalid</a>
								{{else if eq .Status 3}}
									<a class="text-success" href="{{.Verify}}"> <i class="far fa-check-square"></i> Verified</a>
								{{end}}
							</div>
							<div>
							{{if eq .Service "xmpp"}}
								<br/>
								<img src="/qr?s=-2&c={{.Link}}" alt="qrcode" style="width:88px; height:88px">
							{{end}}
							</div>
						</li>
						{{end}}
					</ul>
				</div>
			</div>
		{{else}}
			<div class="card">
				<div class="card-header">Proofs</div>
				<div class="card-body">Loading...</div>
			</div>
			<br/>
		{{end}}
		{{end}}
		<div class="col-lg-8 col-md-12 col-sm-12 col-xs-12">
			<div class="card">
				<div class="card-header">Public Key</div>
				<div class="card-body scroll">
					<pre><code>{{.Entity.ArmorText}}</code></pre>
				</div>
			</div>
		</div>
	</div>
</div>
{{end}}
`

var homeMKDN = `
## About Keyproofs

KeyProofs is a server side version of Keyoxide. There is no JavaScript executed on this page and resourcesKeys are looked up via [Web key directory](https://datatracker.ietf.org/doc/draft-koch-openpgp-webkey-service/)
or from <https://keys.openpgp.org/>.


### Decentralized online identity proofs

- You decide which accounts are linked together
- You decide where this data is stored
- KeyProofs does not store your identity data on its servers
- KeyProofs merely verifies the identity proofs and displays them

### Empowering the internet citizen

- A verified identity proof proves ownership of an account and builds trust
- No bad actor can impersonate you as long as your accounts aren't compromised
- Your online identity data is safe from greedy internet corporations

### User-centric platform

- KeyProofs generates QR codes that integrate with OpenKeychain and Conversations
- KeyProofs fetches the key wherever the user decides to store it
- KeyProofs is self-hostable, meaning you could put it on any server you trust

### Secure and privacy-friendly

- KeyProofs doesn't want your personal data, track you or show you ads
- KeyProofs relies on OpenPGP, a widely used public-key cryptography standard (RFC-4880)
- Cryptographic operations are performed on server.
`
