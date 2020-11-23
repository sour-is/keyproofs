package keyproofs

type page struct {
	Entity *Entity
	Style  *Style
	Proofs *Proofs

	HasProofs  bool
	IsComplete bool
	Err        error
}

var pageTPL = `
<html>
<head>
	{{if not .IsComplete}}<meta http-equiv="refresh" content="1">{{end}}
	<script src="https://pagecdn.io/lib/font-awesome/5.14.0/js/fontawesome.min.js"                   crossorigin="anonymous" integrity="sha256-dNZKI9qQEpJG03MLdR2Rg9Dva1o+50fN3zmlDP+3I+Y="></script>

	<link href="https://pagecdn.io/lib/bootstrap/4.5.1/css/bootstrap.min.css"       rel="stylesheet" crossorigin="anonymous" integrity="sha256-VoFZSlmyTXsegReQCNmbXrS4hBBUl/cexZvPmPWoJsY=" >
	<link href="https://pagecdn.io/lib/font-awesome/5.14.0/css/fontawesome.min.css" rel="stylesheet" crossorigin="anonymous" integrity="sha256-7YMlwkILTJEm0TSengNDszUuNSeZu4KTN3z7XrhUQvc=" >
	<link href="https://pagecdn.io/lib/font-awesome/5.14.0/css/solid.min.css"       rel="stylesheet" crossorigin="anonymous" integrity="sha256-s0DhrAmIsT5gZ3X4f+9wIXUbH52CMiqFAwgqCmdPoec=" >
	<link href="https://pagecdn.io/lib/font-awesome/5.14.0/css/regular.min.css"     rel="stylesheet" crossorigin="anonymous" integrity="sha256-FAKIbnpfWhK6v5Re+NAi9n+5+dXanJvXVFohtH6WAuw=" >
	<link href="https://pagecdn.io/lib/font-awesome/5.14.0/css/brands.min.css"      rel="stylesheet" crossorigin="anonymous" integrity="sha256-xN44ju35FR+kTO/TP/UkqrVbM3LpqUI1VJCWDGbG1ew=" >

{{ with .Style }}
	<style>
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

		@media only screen and (max-width: 768px) {
			 .center-xs { text-align: center; width: 100% }
			 .center-sm { text-align: center; width: 100% }
			 .center-md { text-align: center; width: 100% }
			 h1, h2, h3, h4, h5, h6, .lead  { font-size: 75% }
			}

		 @media only screen and (max-width: 576px) {
			.center-xs { text-align: center; width: 100% }
			.center-sm { text-align: center; width: 100% }
			.center-md { text-align: center; width: 100% }
			h1, h2, h3, h4, h5, h6, .lead  { font-size: 75% }
		}

		@media only screen and (max-width: 0) {
			.center-xs { text-align: center; width: 100% }
			.center-sm { text-align: center; width: 100% }
			.center-md { text-align: center; width: 100% }
			h1, h2, h3, h4, h5, h6, .lead { font-size: 60% }
		}

	</style>
{{end}}
</head>

<body>
	<div class="container">
	<div class="card">
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
						<div class="d-flex w-100 justify-content-between">
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
							<img src="/qr?s=-2&c={{.Link}}" alt="qrcode" style="width:88px; height:88px">
						{{end}}
						</div>
						</div>
					</li>
					{{end}}
				</ul>
			</div>
			<br/>
		{{else}}
			<div class="card">
				<div class="card-header">Proofs</div>
				<div class="card-body">Loading...</div>
			</div>
			<br/>
		{{end}}
		{{end}}
		</div>

		<div class="card-footer text-muted text-center">
			&copy; 2020 Sour.is | <a href="/id/me@sour.is">About me</a> | <a href="https://github.com/sour-is/keyproofs">GitHub</a> | Inspired by <a href="https://keyoxide.org/">keyoxide</a>
		</div>
	</div>
	</div>
</body>
</html>
`
