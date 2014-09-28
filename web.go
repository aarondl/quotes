package quotes

import (
	"html/template"
	"log"
	"net/http"
	"time"
)

var tmpl = template.Must(template.New("quotes").Funcs(template.FuncMap{
	"fmtDate": func(date time.Time) string {
		return date.Format("2006-02-01 15:04:05")
	},
}).Parse(index))

// StartServer starts a webserver to listen on.
func (q *QuoteDB) StartServer(address string) {
	go func() {
		mux := http.NewServeMux()
		mux.HandleFunc("/", q.quotesRoot)
		http.ListenAndServe(address, mux)
	}()
}

func (q *QuoteDB) quotesRoot(w http.ResponseWriter, r *http.Request) {
	quotes, err := q.GetAll()
	if err != nil {
		w.WriteHeader(500)
		log.Println("Failed to get all the quotes:", err)
		return
	}

	data := struct {
		NQuotes int
		Quotes  []Quote
	}{
		q.NQuotes(),
		quotes,
	}

	if err = tmpl.Execute(w, data); err != nil {
		w.WriteHeader(500)
		log.Println("Failed to execute template:", err)
		return
	}
}

const index = `<!DOCTYPE html>
<html>
  <head>
    <title>Quotes</title>
    <link href="https://fonts.googleapis.com/css?family=Lato" rel="stylesheet" type="text/css">
    <style>
    body, html {
      font-size:62.5%;
      margin-top: 50px;
      font-family: 'Lato', sans-serif;
      color: #BEE9C8;
      background-color: #081710;
    }

    .container {
      width: 80%;
      margin: 0 auto;
      font-size: 1.4rem;
    }

    .quotes {
      background-color: rgba(0,0,0,0.3);
      box-shadow: 0px 0px 10px 0px rgba(0,0,0,0.6);
      border-radius: 3px;
    }

    h1 {
      font-size: 2.6rem;
      padding: 0;
      margin: 0;
      padding-bottom: 1rem;
    }

    table thead tr td {
      font-weight: bold;
      border-bottom: solid 1px rgba(255,255,255,0.1);
      background-color: rgba(255,255,255,0.1);
    }

    table tbody tr td {
      vertical-align: top;
      border-bottom: solid 1px rgba(0,0,0,0.1);
    }

    table tbody tr:nth-child(2) td {
      background-color: rgba(0,0,0,0.35);
    }

    table tbody tr:hover {
      background-color: rgba(255,255,255,0.1);
    }

    table {
      width: 100%;
      border-collapse: collapse;
    }

    table .id {
      padding: 0 8px;
      max-width: 50px;
      width: 20px;
    }

    table .author {
      padding: 0 4px;
      max-width: 100px;
      width: 60px;
    }

    table .quote {
    }

    table .date {
      width: 140px;
      max-width: 140px;
    }

    .footer {
      margin-top: 20px;
      text-align: center;
    }
  </style>
  </head>
  <body>
    {{if .Quotes}}
    <div class="container">
      <h1>Quotes</h1>
      <div class="quotes">
        <table>
          <thead>
            <tr>
              <td class="id">ID</td>
              <td class="quote">Quote</td>
              <td class="author">Author</td>
              <td class="date">Date</td>
            </tr>
          </thead>
          <tbody>
            {{range .Quotes}}
            <tr>
              <td class="id">{{.ID}}</td>
              <td class="quote">{{.Quote}}</td>
              <td class="author">{{.Author}}</td>
              <td class="date">{{fmtDate .Date}}</td>
            </tr>
			{{end}}
          </tbody>
        </table>
      </div>
      {{if .NQuotes}}
      <div class="footer">
        {{.NQuotes}} quotes.
      </div>
      {{end}}
      {{else}}
        <center><span>There are no quotes yet.</center></span>
      {{end}}
    </div>
  </body>
</html>`
