package mail

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Regression corpus of real-world and broken messages in testdata/mime.
// Every message a user reports as badly displayed must be added here.

type mimeCase struct {
	subject   string   // exact decoded subject ("" = don't check)
	text      string   // substring expected in the text body
	html      string   // substring expected in the HTML body
	files     []string // expected attachment names, in order
	inlineCID string   // expected inline part Content-ID
}

var mimeCorpus = map[string]mimeCase{
	"01-windows-1252.eml":           {subject: "Orçamento “final”", text: "€1.200 – prazo “sexta”"},
	"02-iso-2022-jp.eml":            {subject: "会議の予定", text: "明日の会議は10時です。"},
	"03-shift-jis.eml":              {subject: "請求書", text: "お支払いをお願いします。"},
	"04-gb2312.eml":                 {subject: "你好", text: "欢迎使用邮件。"},
	"05-koi8-r.eml":                 {subject: "Привет", text: "Добрый день, коллеги."},
	"06-utf8-bom.eml":               {subject: "Com BOM", text: "Olá com BOM"},
	"07-unknown-charset.eml":        {subject: "Charset desconhecido", text: "texto simples ascii"},
	"08-rfc2231-filename.eml":       {text: "corpo", files: []string{"relatório final.pdf"}},
	"09-encoded-word-filename.eml":  {text: "corpo", files: []string{"cotação.pdf"}},
	"10-attachment-no-name.eml":     {text: "corpo", files: []string{"anexo-2"}},
	"11-nested-related.eml":         {text: "versao texto", html: "versao html", files: []string{"logo1", "dados.csv"}, inlineCID: "logo1"},
	"12-truncated.eml":              {text: "comeco do texto"},
	"13-invalid-base64.eml":         {text: "texto ok"},
	"14-forwarded-rfc822.eml":       {text: "segue abaixo", files: []string{"original.eml"}},
	"15-folded-subject.eml":         {subject: "Um assunto muito longo que foi quebrado em várias linhas", text: "ok"},
	"16-lf-only.eml":                {subject: "Quebras LF", text: "linha 2"},
	"17-empty-body.eml":             {subject: "Vazio"},
	"18-deep-nesting.eml":           {subject: "Aninhamento profundo"},
	"19-tnef.eml":                   {text: "veja anexo", files: []string{"winmail.dat"}},
	"20-raw-utf8-headers.eml":       {subject: "Convite: Reunião às 10h", text: "Reunião às 10h"},
}

func TestParse_MIMECorpus(t *testing.T) {
	files, _ := filepath.Glob("testdata/mime/*.eml")
	if len(files) != len(mimeCorpus) {
		t.Fatalf("corpus has %d files but %d expectations", len(files), len(mimeCorpus))
	}
	for _, f := range files {
		name := filepath.Base(f)
		want, ok := mimeCorpus[name]
		if !ok {
			t.Errorf("%s: no expectation", name)
			continue
		}
		t.Run(name, func(t *testing.T) {
			fh, err := os.Open(f)
			if err != nil {
				t.Fatal(err)
			}
			defer fh.Close()
			p, err := Parse(fh)
			if err != nil {
				t.Fatalf("parse failed: %v", err)
			}
			if want.subject != "" && p.Subject != want.subject {
				t.Errorf("subject = %q, want %q", p.Subject, want.subject)
			}
			if want.text != "" && !strings.Contains(p.Text, want.text) {
				t.Errorf("text = %q, want it to contain %q", p.Text, want.text)
			}
			if want.html != "" && !strings.Contains(p.HTML, want.html) {
				t.Errorf("html = %q, want it to contain %q", p.HTML, want.html)
			}
			var got []string
			for _, a := range p.Attachments {
				got = append(got, a.Name)
			}
			if want.files != nil && strings.Join(got, "|") != strings.Join(want.files, "|") {
				t.Errorf("attachments = %v, want %v", got, want.files)
			}
			if want.inlineCID != "" && p.Part(0, want.inlineCID) == nil {
				t.Errorf("inline part %q not found", want.inlineCID)
			}
			if strings.ContainsRune(p.Text, rune(0xFEFF)) {
				t.Error("byte order mark left in text")
			}
		})
	}
}

func FuzzParse(f *testing.F) {
	files, _ := filepath.Glob("testdata/mime/*.eml")
	for _, file := range files {
		b, _ := os.ReadFile(file)
		f.Add(b)
	}
	f.Fuzz(func(t *testing.T, b []byte) {
		p, err := Parse(strings.NewReader(string(b)))
		if err == nil {
			_ = Sanitize(p.HTML, opts) // must never panic either
		}
	})
}
