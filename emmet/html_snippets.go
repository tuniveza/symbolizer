package emmet

import "strings"

// mkSnippetSrc is Emmet's HTML snippet table. "a|b" names are aliases.
// Each value is itself an abbreviation (resolved again, so "input:email"
// becomes "inp[type=email]" becomes input[name id] + input[type]).
var mkSnippetSrc = map[string]string{
	"a":                              "a[href]",
	"a:blank":                        "a[href='http://${0}' target='_blank' rel='noopener noreferrer']",
	"a:link":                         "a[href='http://${0}']",
	"a:mail":                         "a[href='mailto:${0}']",
	"a:tel":                          "a[href='tel:+${0}']",
	"abbr":                           "abbr[title]",
	"acr|acronym":                    "acronym[title]",
	"base":                           "base[href]/",
	"basefont":                       "basefont/",
	"br":                             "br/",
	"frame":                          "frame/",
	"hr":                             "hr/",
	"bdo":                            "bdo[dir]",
	"bdo:r":                          "bdo[dir=rtl]",
	"bdo:l":                          "bdo[dir=ltr]",
	"col":                            "col/",
	"link":                           "link[rel=stylesheet href]/",
	"link:css":                       "link[href='${1:style}.css']",
	"link:print":                     "link[href='${1:print}.css' media=print]",
	"link:favicon":                   "link[rel='shortcut icon' type=image/x-icon href='${1:favicon.ico}']",
	"link:mf|link:manifest":          "link[rel='manifest' href='${1:manifest.json}']",
	"link:touch":                     "link[rel=apple-touch-icon href='${1:favicon.png}']",
	"link:rss":                       "link[rel=alternate type=application/rss+xml title=RSS href='${1:rss.xml}']",
	"link:atom":                      "link[rel=alternate type=application/atom+xml title=Atom href='${1:atom.xml}']",
	"link:im|link:import":            "link[rel=import href='${1:component}.html']",
	"meta":                           "meta/",
	"meta:utf":                       "meta[http-equiv=Content-Type content='text/html;charset=UTF-8']",
	"meta:vp":                        "meta[name=viewport content='width=device-width, initial-scale=1.0']",
	"meta:compat":                    "meta[http-equiv=X-UA-Compatible content='${1:IE=7}']",
	"meta:edge":                      "meta:compat[content='${1:ie=edge}']",
	"meta:redirect":                  "meta[http-equiv=refresh content='0; url=${1:http://example.com}']",
	"meta:refresh":                   "meta[http-equiv=refresh content='${1:5}']",
	"meta:kw":                        "meta[name=keywords content]",
	"meta:desc":                      "meta[name=description content]",
	"style":                          "style",
	"script":                         "script",
	"script:src":                     "script[src]",
	"script:module":                  "script[type=module src]",
	"img":                            "img[src alt]/",
	"img:s|img:srcset":               "img[srcset src alt]",
	"img:z|img:sizes":                "img[sizes srcset src alt]",
	"picture":                        "picture",
	"src|source":                     "source/",
	"src:sc|source:src":              "source[src type]",
	"src:s|source:srcset":            "source[srcset]",
	"src:t|source:type":              "source[srcset type='${1:image/}']",
	"src:z|source:sizes":             "source[sizes srcset]",
	"src:m|source:media":             "source[media='(${1:min-width: })' srcset]",
	"src:mt|source:media:type":       "source:media[type='${2:image/}']",
	"src:mz|source:media:sizes":      "source:media[sizes srcset]",
	"src:zt|source:sizes:type":       "source[sizes srcset type='${1:image/}']",
	"iframe":                         "iframe[src frameborder=0]",
	"embed":                          "embed[src type]/",
	"object":                         "object[data type]",
	"param":                          "param[name value]/",
	"map":                            "map[name]",
	"area":                           "area[shape coords href alt]/",
	"area:d":                         "area[shape=default]",
	"area:c":                         "area[shape=circle]",
	"area:r":                         "area[shape=rect]",
	"area:p":                         "area[shape=poly]",
	"form":                           "form[action]",
	"form:get":                       "form[method=get]",
	"form:post":                      "form[method=post]",
	"label":                          "label[for]",
	"input":                          "input[type=${1:text}]/",
	"inp":                            "input[name=${1} id=${1}]",
	"input:h|input:hidden":           "input[type=hidden name]",
	"input:t|input:text":             "inp[type=text]",
	"input:search":                   "inp[type=search]",
	"input:email":                    "inp[type=email]",
	"input:url":                      "inp[type=url]",
	"input:p|input:password":         "inp[type=password]",
	"input:datetime":                 "inp[type=datetime]",
	"input:date":                     "inp[type=date]",
	"input:datetime-local":           "inp[type=datetime-local]",
	"input:month":                    "inp[type=month]",
	"input:week":                     "inp[type=week]",
	"input:time":                     "inp[type=time]",
	"input:tel":                      "inp[type=tel]",
	"input:number":                   "inp[type=number]",
	"input:color":                    "inp[type=color]",
	"input:c|input:checkbox":         "inp[type=checkbox]",
	"input:r|input:radio":            "inp[type=radio]",
	"input:range":                    "inp[type=range]",
	"input:f|input:file":             "inp[type=file]",
	"input:s|input:submit":           "input[type=submit value]",
	"input:i|input:image":            "input[type=image src alt]",
	"input:b|input:btn|input:button": "input[type=button value]",
	"input:reset":                    "input:button[type=reset]",
	"isindex":                        "isindex/",
	"select":                         "select[name=${1} id=${1}]",
	"select:d|select:disabled":       "select[disabled.]",
	"opt|option":                     "option[value]",
	"textarea":                       "textarea[name=${1} id=${1} cols=${2:30} rows=${3:10}]",
	"marquee":                        "marquee[behavior direction]",
	"menu:c|menu:context":            "menu[type=context]",
	"menu:t|menu:toolbar":            "menu[type=toolbar]",
	"video":                          "video[src]",
	"audio":                          "audio[src]",
	"html:xml":                       "html[xmlns=http://www.w3.org/1999/xhtml]",
	"keygen":                         "keygen/",
	"command":                        "command/",
	"btn:s|button:s|button:submit":   "button[type=submit]",
	"btn:r|button:r|button:reset":    "button[type=reset]",
	"btn:d|button:d|button:disabled": "button[disabled.]",
	"fst:d|fset:d|fieldset:d|fieldset:disabled": "fieldset[disabled.]",

	"bq":    "blockquote",
	"fig":   "figure",
	"figc":  "figcaption",
	"pic":   "picture",
	"ifr":   "iframe",
	"emb":   "embed",
	"obj":   "object",
	"cap":   "caption",
	"colg":  "colgroup",
	"fst":   "fieldset",
	"btn":   "button",
	"optg":  "optgroup",
	"tarea": "textarea",
	"leg":   "legend",
	"sect":  "section",
	"art":   "article",
	"hdr":   "header",
	"ftr":   "footer",
	"adr":   "address",
	"dlg":   "dialog",
	"str":   "strong",
	"prog":  "progress",
	"mn":    "main",
	"tem":   "template",
	"fset":  "fieldset",
	"datal": "datalist",
	"kg":    "keygen",
	"out":   "output",
	"det":   "details",
	"sum":   "summary",
	"cmd":   "command",
	"data":  "data[value]",
	"meter": "meter[value]",
	"time":  "time[datetime]",

	"ri:d|ri:dpr":      "img:s",
	"ri:v|ri:viewport": "img:z",
	"ri:a|ri:art":      "pic>src:m+img",
	"ri:t|ri:type":     "pic>src:t+img",

	"!!!":      "{<!DOCTYPE html>}",
	"!!!4t":    "{<!DOCTYPE HTML PUBLIC \"-//W3C//DTD HTML 4.01 Transitional//EN\" \"http://www.w3.org/TR/html4/loose.dtd\">}",
	"!!!4s":    "{<!DOCTYPE HTML PUBLIC \"-//W3C//DTD HTML 4.01//EN\" \"http://www.w3.org/TR/html4/strict.dtd\">}",
	"!!!xt":    "{<!DOCTYPE html PUBLIC \"-//W3C//DTD XHTML 1.0 Transitional//EN\" \"http://www.w3.org/TR/xhtml1/DTD/xhtml1-transitional.dtd\">}",
	"!!!xs":    "{<!DOCTYPE html PUBLIC \"-//W3C//DTD XHTML 1.0 Strict//EN\" \"http://www.w3.org/TR/xhtml1/DTD/xhtml1-strict.dtd\">}",
	"!!!xxs":   "{<!DOCTYPE html PUBLIC \"-//W3C//DTD XHTML 1.1//EN\" \"http://www.w3.org/TR/xhtml11/DTD/xhtml11.dtd\">}",
	"doc":      "html[lang=${lang}]>(head>meta[charset=${charset}]+meta:vp+title{${1:Document}})+body",
	"doc4":     "html>(head>meta[http-equiv=\"Content-Type\" content=\"text/html;charset=${charset}\"]+title{${1:Document}})+body",
	"!|html:5": "!!!+doc",
	"html:4t":  "!!!4t+doc4[lang=${lang}]",
	"html:4s":  "!!!4s+doc4[lang=${lang}]",
	"html:xt":  "!!!xt+doc4[xmlns=http://www.w3.org/1999/xhtml xml:lang=${lang}]",
	"html:xs":  "!!!xs+doc4[xmlns=http://www.w3.org/1999/xhtml xml:lang=${lang}]",
	"html:xxs": "!!!xxs+doc4[xmlns=http://www.w3.org/1999/xhtml xml:lang=${lang}]",

	"c":       "{<!-- ${0} -->}",
	"cc:ie":   "{<!--[if IE]>${0}<![endif]-->}",
	"cc:noie": "{<!--[if !IE]><!-->${0}<!--<![endif]-->}",

	"ol+":             "ol>li",
	"ul+":             "ul>li",
	"dl+":             "dl>dt+dd",
	"map+":            "map>area",
	"table+":          "table>tr>td",
	"tr+":             "tr>td",
	"select+":         "select>option",
	"optgroup+|optg+": "optgroup>option",
	"colgroup+|colg+": "colgroup>col",
}

// mkSnippets is mkSnippetSrc with the aliases split out.
var mkSnippets = func() map[string]string {
	m := map[string]string{}
	for k, v := range mkSnippetSrc {
		for _, name := range strings.Split(k, "|") {
			m[name] = v
		}
	}
	return m
}()

// mkVariables fill ${name} in snippets.
var mkVariables = map[string]string{
	"lang":    "en",
	"locale":  "en-US",
	"charset": "UTF-8",
}

func mkSet(words string) map[string]bool {
	m := map[string]bool{}
	for _, w := range strings.Fields(words) {
		m[w] = true
	}
	return m
}

// mkTags is every HTML element name (plus a few old ones Emmet knows).
var mkTags = mkSet(`a abbr acronym address applet area article aside audio b base basefont bdi bdo
big blockquote body br button canvas caption center cite code col colgroup command data datalist
dd del details dfn dialog dir div dl dt em embed fieldset figcaption figure font footer form frame
frameset h1 h2 h3 h4 h5 h6 head header hgroup hr html i iframe img input ins isindex kbd keygen
label legend li link main map mark marquee math menu meta meter nav noframes noscript object ol
optgroup option output p param picture pre progress q rp rt ruby s samp script search section
select slot small source span strike strong style sub summary sup svg table tbody td template
textarea tfoot th thead time title tr track tt u ul var video wbr`)

// mkCommonTags are the tags a typo may be fixed to.
var mkCommonTags = mkSet(`div span section article header footer nav main aside ul ol li table
tr td th thead tbody tfoot form input button label select option textarea img a p h1 h2 h3 h4 h5
h6 strong em small pre code blockquote figure figcaption video audio canvas iframe script style
link meta title head body html br hr dl dt dd caption fieldset legend picture source template
details summary dialog address`)

// mkTypoStop are English words one slip away from a tag: never "fixed".
var mkTypoStop = mkSet(`from meat able side tile cable fable gable sable stable tablet lapel
abel heard heady bodies coder coded codes model nodes modes titles`)

// mkInline are inline-level elements (Emmet's inlineElements option).
var mkInline = mkSet(`a abbr acronym applet b basefont bdo big br button cite code del dfn em
font i iframe img input ins kbd label map object q s samp select small span strike strong sub
sup textarea tt u var`)

// mkVoid are elements that never have content or a closing tag in HTML.
var mkVoid = mkSet(`area base basefont br col command embed frame hr img input isindex keygen
link meta param source track wbr`)

// mkBoolAttrs are attributes written without a value in HTML.
var mkBoolAttrs = mkSet(`contenteditable seamless async autofocus autoplay checked controls
defer disabled formnovalidate hidden ismap loop multiple muted novalidate readonly required
reversed selected typemustmatch`)

// mkImplicit is the default child tag inside a parent (else span in inline, else div).
var mkImplicit = map[string]string{
	"p": "span", "ul": "li", "ol": "li", "table": "tr", "tr": "td", "tbody": "tr",
	"thead": "tr", "tfoot": "tr", "colgroup": "col", "select": "option",
	"optgroup": "option", "audio": "source", "video": "source", "object": "param",
	"map": "area",
}

// mkJSWords are words that are tags or snippets but in JSX/JS code are
// much more often plain identifiers, so Tab does not expand them alone.
var mkJSWords = mkSet(`var let const new map set object data time output source src track
template menu title del label select option input link meta style script code small big str
out sum det art cap col prog cmd obj emb kg mn tem area param main summary details address
abbr dialog base frame keygen command search slot ruby mark font center dir picture embed
meter progress`)
