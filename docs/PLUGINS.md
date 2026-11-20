# Criando plugins para o meofishes

Tudo que o meofishes toca, mostra ou reorganiza vem de plugins. Um plugin é
uma pasta com um `plugin.json`. Ela é instalada em
`~/.local/share/meofishes/plugins/<id>/`, ou pela interface em **Plugins →
Instalar .zip / pasta / URL**.

```
meu-plugin/
├── plugin.json      ← obrigatório
└── main.js          ← só para runtime "script"
```

Teste sem abrir a interface:

```sh
meofishes check ./meu-plugin             # valida o plugin.json
meofishes search meu-plugin "daft punk"  # roda a busca e resolve a 1ª faixa
meofishes lyrics "Artista" "Música"      # testa a cadeia de letras
meofishes modules                        # bibliotecas Go disponíveis
```

## Tipos (o que o plugin fornece)

| tipo     | para quê                                                  |
|----------|-----------------------------------------------------------|
| `source` | fonte de música: busca, página inicial, stream            |
| `lyrics` | fonte de letras (entra na fila de prioridade das letras)  |
| `layout` | muda a posição das regiões da interface                   |
| `hook`   | reage a eventos do player (scrobble, rich presence…)      |

Um plugin pode ter vários tipos, por exemplo `["source", "lyrics"]`.

## Runtimes (como o plugin roda)

| runtime       | quando usar                                                          |
|---------------|----------------------------------------------------------------------|
| `declarative` | a API é HTTP + JSON simples: você só descreve endpoints e campos    |
| `script`      | precisa de lógica: JavaScript com as bibliotecas Go do meofishes     |
| `process`     | quer usar qualquer biblioteca Go (ou outra linguagem): um executável |
| *(nenhum)*    | plugins só de `layout`                                               |

## Campos do plugin.json

```jsonc
{
  "id": "meu-plugin",              // minúsculas, números, . _ -
  "name": "Meu Plugin",
  "version": "1.0.0",
  "author": "você",
  "description": "O que ele faz",
  "homepage": "https://…",
  "types": ["source"],
  "runtime": "declarative",
  "entry": "main.js",              // script: arquivo JS. process: comando
  "build": "go build -o bin .",    // process: compila na instalação (com confirmação)
  "modules": ["http", "ytdlp"],    // bibliotecas Go pedidas ao meofishes
  "hosts": ["api.exemplo.com", "*.cdn.exemplo.com"], // limita o módulo http
  "settings": [                    // viram um formulário em "Configurar"
    { "key": "token", "label": "Token", "type": "password" },
    { "key": "site", "type": "choice", "choices": ["a", "b"], "default": "a" },
    { "key": "hq", "type": "bool", "default": "true" }
  ],
  "events": ["track.start"]        // hook: eventos que quer receber ("*" = todos)
}
```

Tipos de setting: `string` (padrão), `password`, `bool`, `number`, `choice`, `list`.

## Faixa (Track)

Todos os runtimes devolvem faixas neste formato:

```json
{
  "id": "único dentro do plugin",
  "title": "Música", "artist": "Artista", "album": "Álbum",
  "durationMs": 215000,
  "cover": "https://…/capa.jpg",
  "streamUrl": "https://…/audio.mp3   ou   https://youtube.com/watch?v=…",
  "isrc": "USUG11904206",          // ajuda muito a achar a letra certa
  "videoId": "fHI8X4OXluQ",        // id do YouTube, usado pelo Unison
  "extra": { "qualquer": "coisa" } // volta para você no resolve()
}
```

Se `streamUrl` for uma **página** (YouTube, SoundCloud, Bandcamp…) e não um
arquivo de áudio, devolva `{ "url": …, "page": true }` no resolve. O mpv extrai
o áudio com o yt-dlp.

---

## 1. Plugin declarativo (sem código)

Você descreve onde cada função do app busca os dados. Templates `{{…}}`
funcionam em URLs, cabeçalhos e corpo. Nas URLs os valores são escapados
automaticamente. Filtros: `{{query|raw}}`, `|url`, `|json`, `|lower`, `|upper`.

Variáveis: `query` (busca), `settings.*`, `app.version`, `app.userAgent`,
`track.*` (no resolve) e `item.*` (no map).

```json
{
  "id": "radio-browser",
  "name": "Radio Browser",
  "version": "1.0.0",
  "types": ["source"],
  "runtime": "declarative",
  "settings": [{ "key": "server", "default": "https://de1.api.radio-browser.info" }],
  "source": {
    "baseUrl": "{{settings.server}}",
    "headers": { "Accept": "application/json" },
    "search": {
      "url": "/json/stations/search?name={{query}}&limit=50",
      "items": "$",
      "map": {
        "id": "stationuuid",
        "title": "name",
        "artist": "country || countrycode",
        "cover": "favicon",
        "streamUrl": "url_resolved || url"
      }
    },
    "home":    { "url": "/json/stations/topclick/30", "items": "$", "map": { "…": "…" } },
    "resolve": { "url": "/json/url/{{track.id}}", "path": "url" },
    "resolver": "direct"
  }
}
```

- `items`: caminho até a lista no JSON (`$` = a raiz, `data.results`, `items[0].tracks`).
- `map`: campo da faixa → caminho no item. `a || b` tenta alternativas, `'texto'`
  é literal, e `"https://img/{{item.id}}.jpg"` é um template. Use `durationSec`
  quando a API dá segundos. Campos desconhecidos vão para `extra`.
- `resolve` (opcional): chamada extra para pegar a URL de áudio.
- `resolver`: `direct` (a URL é áudio) ou `ytdlp` (a URL é uma página).

Plugin de letras declarativo:

```json
{
  "id": "lrclib", "name": "LRCLIB", "version": "1.0.0",
  "types": ["lyrics"], "runtime": "declarative",
  "lyrics": {
    "request": { "url": "https://lrclib.net/api/search?track_name={{cleanTitle}}&artist_name={{primaryArtist}}" },
    "path": "[0].syncedLyrics || [0].plainLyrics",
    "format": "auto",
    "credit": "Letras do LRCLIB"
  }
}
```

Variáveis de letras: `title`, `artist`, `album`, `durationSec`, `durationMs`,
`isrc`, `videoId`, `cleanTitle` (sem "(Official Video)" etc.), `primaryArtist`.
Formatos: `ttml`, `lrc` (inclusive LRC com tempo por palavra `<mm:ss.xx>`),
`krc`, `plain`, `auto`.

---

## 2. Plugin script (JavaScript + bibliotecas Go)

O JavaScript roda num motor embutido (goja, ES2015+), de forma **síncrona**:
as chamadas aos módulos simplesmente devolvem o resultado, sem `await`.

```js
const http = require("http");    // precisa estar em "modules"
const util = require("./util");  // outro arquivo da pasta do plugin

exports.search = (query) => {
  const data = http.getJSON("https://api.exemplo.com/search", { query: { q: query } });
  return { tracks: data.items.map(x => ({ id: x.id, title: x.name, artist: x.by, streamUrl: x.mp3 })) };
};
exports.home    = () => ({ tracks: [] });               // opcional
exports.resolve = (track) => track.streamUrl;           // opcional; ou { url, headers, page }
exports.lyrics  = (q) => ({ format: "lrc", content: "[00:01.00]…" }); // tipo lyrics
exports.onEvent = (name, data) => {};                   // tipo hook
```

Globais: `settings` (somente leitura), `app.version`, `app.userAgent`,
`app.plugin`, `console.log/warn/error` (vão para **Ajustes → Registro**).
Exceções viram mensagens de erro na interface.

### Bibliotecas Go disponíveis (`modules`)

| módulo     | funções |
|------------|---------|
| `http`     | `get(url, opts)`, `getJSON(url, opts)`, `post(url, body, opts)`, `postJSON(url, body, opts)`, `request({url, method, body, headers, query, timeout})`, `encode(obj)`. `opts = { headers, query, timeout }`. Resposta: `{ status, ok, headers, body, json }`. Respeita `hosts`. |
| `ytdlp`    | `available()`, `search(query, limite, site)` com `site` = `youtube`/`ytmusic`/`soundcloud`, `info(url)`, `streamUrl(url)`, `playlist(url, limite)`. Precisa do `yt-dlp` instalado. Cobre ~1800 sites. |
| `lavalink` | `connect({host, port, password, secure})` → `{ search(query, prefixo), loadTracks(identificador), decode(encoded), info() }`. Prefixos: `ytsearch`, `ytmsearch`, `scsearch` e, com LavaSrc, `spsearch`, `dzsearch`, `amsearch`. |
| `lyrics`   | `parse(formato, conteúdo)`, `cleanTitle(s)`, `primaryArtist(s)`, `matchScore(query, título, artista, duraçãoMs)`, `decodeKRC(base64)` |
| `crypto`   | `md5`, `sha1`, `sha256`, `hmacSha256(chave, msg)`, `base64Encode`, `base64Decode`, `uuid` |
| `storage`  | `get(k)`, `set(k, v)`, `remove(k)`, `all()`. Persistente, um arquivo por plugin. |

> **Sobre o Lavalink:** o Lavalink resolve e busca faixas, mas só transmite
> áudio para canais de voz do Discord. Para tocar localmente, o plugin devolve
> a `uri` da faixa (a página) e o mpv toca via yt-dlp. Veja `assets/plugins/lavalink`.

---

## 3. Plugin process (Go ou qualquer linguagem)

Um executável que conversa com o meofishes por JSON, uma mensagem por linha, em
stdin/stdout. Em Go, use o SDK (`sdk/sdk.go`, só biblioteca padrão: importe ou
copie para o seu projeto) e **qualquer biblioteca Go que quiser**.

```go
package main

import "meofishes/sdk" // ou uma cópia local do sdk.go

func main() {
	sdk.Serve(sdk.Plugin{
		Search: func(ctx *sdk.Context, q string) (*sdk.SearchResult, error) {
			ctx.Log("buscando %s", q) // aparece no Registro
			return &sdk.SearchResult{Tracks: []sdk.Track{{ID: "1", Title: "Música", StreamURL: "https://…/a.mp3"}}}, nil
		},
		Resolve: func(ctx *sdk.Context, t sdk.Track) (*sdk.Stream, error) {
			return &sdk.Stream{URL: t.StreamURL}, nil
		},
		Lyrics: func(ctx *sdk.Context, q sdk.LyricsQuery) (*sdk.Lyrics, error) { return nil, sdk.ErrNotFound },
		Event:  func(ctx *sdk.Context, name string, payload json.RawMessage) {},
	})
}
```

```json
{ "id": "meu-go", "name": "Meu plugin Go", "version": "1.0.0",
  "types": ["source"], "runtime": "process",
  "entry": "./meu-go", "build": "go build -o meu-go ." }
```

O comando `build` roda uma vez, na instalação, **depois de o usuário confirmar**,
porque ele executa código do plugin. Exemplo completo: `assets/plugins/somafm-go`.

Protocolo (para outras linguagens):

```
→ {"id":1,"method":"initialize","params":{"plugin":"…","version":"…","userAgent":"…","settings":{…},"dataDir":"…"}}
← {"id":1,"result":{"capabilities":["search","resolve","home","open","lyrics","event"]}}
→ {"id":2,"method":"search","params":{"query":"…"}}         ← {"id":2,"result":{"tracks":[…]}}
→ {"id":3,"method":"resolve","params":{"track":{…}}}        ← {"id":3,"result":{"url":"…","page":false}}
→ {"id":4,"method":"lyrics","params":{"query":{…}}}         ← {"id":4,"result":{"format":"ttml","content":"…"}}
→ {"method":"event","params":{"name":"track.start","payload":{…}}}   (sem id = notificação)
→ {"method":"settings","params":{…}}                                 (configurações mudaram)
← {"method":"log","params":{"message":"…"}}
← {"id":5,"error":"not found"}                                        (sem resultado)
```

---

## Plugins de layout

A janela é montada a partir de uma árvore de caixas (`box`), divisórias
redimensionáveis (`split`) e **regiões**:

`sidebar`, `pages`, `nowplaying`, `lyrics`, `queue`, `playerbar`, `cover`, `spacer`

```json
{
  "id": "layout-cinema", "name": "Layout: Cinema", "version": "1.0.0",
  "types": ["layout"],
  "layout": {
    "box": "v", "spacing": 0,
    "children": [
      { "region": "playerbar" },
      { "split": "h", "stretch": 1, "sizes": [220, 1000], "children": [
        { "region": "sidebar", "minWidth": 190 },
        { "split": "v", "children": [ { "region": "pages", "stretch": 1 }, { "region": "lyrics", "minHeight": 160 } ] }
      ]}
    ]
  }
}
```

Atributos dos nós: `box` (`h`/`v`), `split` (`h`/`v`), `region`, `name` (vira
`#name` no CSS), `stretch`, `sizes`, `spacing`, `margins` `[e, c, d, b]`,
`minWidth`, `maxWidth`, `minHeight`, `maxHeight`, `children`.
Escolha o layout em **Plugins → Layout da interface**. Se o layout esquecer
`pages`, o meofishes adiciona essa região sozinho, para você não perder acesso
aos Plugins e Temas.

## Eventos (hooks)

| evento            | payload                          |
|-------------------|----------------------------------|
| `track.start`     | `{ track }`                      |
| `track.end`       | `{ track, positionMs }`          |
| `playback.pause`  | —                                |
| `playback.resume` | —                                |
| `playback.seek`   | `{ positionMs }`                 |
| `lyrics.found`    | `{ track, provider, sync }`      |
| `theme.change`    | `{ theme }`                      |

Use `"events": ["track.*"]` para receber todos os eventos de faixa.

## Distribuindo

Compacte a pasta num `.zip` (o `plugin.json` pode estar na raiz ou numa única
subpasta) e publique. Para aparecer na lista **Catálogo**, adicione uma entrada
num `index.json` e peça para o usuário colocar a URL em **Ajustes → Catálogos**:

```json
{
  "name": "Meu catálogo",
  "entries": [
    { "kind": "plugin", "id": "meu-plugin", "name": "Meu Plugin", "version": "1.0.0",
      "author": "você", "description": "…", "types": ["source"], "runtime": "script",
      "url": "meu-plugin-1.0.0.zip" },
    { "kind": "theme", "id": "meu-tema", "name": "Meu Tema", "version": "1.0.0",
      "preview": { "background": "#111", "surface": "#222", "accent": "#f0a", "highlight": "#fcd", "text": "#fff" },
      "url": "https://exemplo.com/meu-tema.zip" }
  ]
}
```

URLs relativas são resolvidas em relação ao `index.json`.
