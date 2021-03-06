package quotes

import (
	"bytes"
	"fmt"
	"html/template"
	"io"
	"log"
	"net/http"
	"net/url"
	"regexp"
	"sort"
	"time"

	"golang.org/x/crypto/bcrypt"
)

var rgxSplitQuote = regexp.MustCompile(`<[^>]+>[^<]+`)

func splitEm(q string) []string {
	matches := rgxSplitQuote.FindAllString(q, -1)
	if matches != nil {
		return matches
	}

	return []string{q}
}

var tmpl = template.Must(template.New("quotes").Funcs(template.FuncMap{
	"fmtDate": func(date time.Time) string {
		return date.Format("2006-01-02 15:04:05")
	},
	"sub": func(a, b int) string {
		return fmt.Sprint(a - b)
	},
	"splitEm": splitEm,
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
	if len(q.webuser) != 0 || len(q.webhash) != 0 {
		user, pwd, ok := r.BasicAuth()
		if !ok || q.webuser != user || nil != bcrypt.CompareHashAndPassword(q.webhash, []byte(pwd)) {
			w.Header().Set("WWW-Authenticate", "Basic realm=Quotes")
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
	}

	if r.URL.Path != "/" {
		w.WriteHeader(http.StatusNotFound)
		return
	}

	showAll := false
	voteSort := false
	query := r.URL.Query()
	if query.Get("all") == "true" {
		showAll = true
	}
	if query.Get("votesort") == "true" {
		voteSort = true
	}

	quotes, err := q.GetAll(!showAll)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		log.Println("Failed to get all the quotes:", err)
		return
	}

	allQuery := cloneQuery(query)
	allQuery.Set("all", "true")
	votesortQuery := cloneQuery(query)
	votesortQuery.Set("votesort", "true")

	data := struct {
		NQuotes      int
		Quotes       []Quote
		AllHref      template.HTMLAttr
		VotesortHref template.HTMLAttr
	}{
		NQuotes:      len(quotes),
		Quotes:       quotes,
		AllHref:      template.HTMLAttr(fmt.Sprintf(`href="/?%s"`, allQuery.Encode())),
		VotesortHref: template.HTMLAttr(fmt.Sprintf(`href="/?%s"`, votesortQuery.Encode())),
	}

	if voteSort {
		sort.Slice(data.Quotes, func(i, j int) bool {
			iquote := data.Quotes[i]
			jquote := data.Quotes[j]
			ivotes := iquote.Upvotes - iquote.Downvotes
			jvotes := jquote.Upvotes - jquote.Downvotes

			return ivotes > jvotes || (ivotes == jvotes && iquote.ID > jquote.ID)
		})
	}

	buf := &bytes.Buffer{}
	if err = tmpl.Execute(buf, data); err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		log.Println("Failed to execute template:", err)
		return
	}

	_, _ = io.Copy(w, buf)
}

func cloneQuery(vals url.Values) url.Values {
	clone := make(url.Values)
	for k, v := range vals {
		vals := make([]string, len(v))
		copy(vals, v)
		clone[k] = v
	}

	return clone
}

const index = `<!DOCTYPE html>
<html>
  <head>
    <title>Quotes</title>
    <link href="https://fonts.googleapis.com/css?family=Lato" rel="stylesheet" type="text/css">
    <style>
    body, html {
      font-size: 62.5%;
      margin-top: 50px;
      font-family: 'Lato', sans-serif;
      color: #AAAFB6;
      background-color: #5F6B7B;
    }

    a {
      color: #294977;
      text-decoration: none;
    }

    a:hover {
      text-decoration: underline;
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

    table tbody tr:nth-child(2n) td {
      background-color: rgba(0,0,0,0.05);
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

    table .votes {
      width: 50px;
      max-width: 60px;
    }

    table .upvotes {
      width: 50px;
      max-width: 60px;
    }

    table .downvotes {
      width: 50px;
      max-width: 60px;
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
      <h1>Quotes (<a {{.AllHref}}>show all</a>) (<a {{.VotesortHref}}>votesort</a>)</h1>
      <div class="quotes">
        <table>
          <thead>
            <tr>
              <td class="id">ID</td>
              <td class="votes">Votes</td>
              <td class="quote">Quote</td>
              <td class="author">Author</td>
              <td class="date">Date</td>
              <td class="upvotes">Up</td>
              <td class="downvotes">Down</td>
            </tr>
          </thead>
          <tbody>
            {{range .Quotes}}
            <tr>
              <td class="id">{{.ID}}</td>
              <td class="votes">{{sub .Upvotes .Downvotes}}</td>
              <td class="quote">{{range $i, $q := .Quote | splitEm}}{{if not (eq 0 $i)}}<br>{{end}}{{$q}}{{end}}</td>
              <td class="author">{{.Author}}</td>
              <td class="date">{{fmtDate .Date}}</td>
              <td class="upvotes">{{.Upvotes}}</td>
              <td class="downvotes">{{.Downvotes}}</td>
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
        <center><span style="font-size: 2rem;">There are no quotes yet (<a {{.AllHref}}>show all</a>).</center></span>
      {{end}}
    </div>
  </body>
</html>`
