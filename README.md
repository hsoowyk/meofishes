# meofishes 🐟

Um player de música em Go + Qt5 em que **toda a música vem de plugins**: quer
ouvir de algum lugar? Instale o plugin e pronto.

- **Plugins de 4 tipos**: fontes de música, letras, layout da interface e eventos.
- **3 jeitos de escrever plugins**: só JSON (endpoints + mapeamento de campos),
  JavaScript com bibliotecas Go do app (`http`, `ytdlp`, `lavalink`, `lyrics`,
  `crypto`, `storage`) ou um programa em Go (ou outra linguagem) com o SDK.
- **Letras sincronizadas por palavra** por padrão, no plugin *Meo Lyrics*:
  Lyprio → BetterLyrics → lrc.red → Unison → Paxsenix → Kugou.
- **Interface 100% por CSS**: cores, fontes, animações, letras, capas, barras e
  fundo. Temas são instalados e aplicados pela interface, com catálogo para
  baixar. Tema padrão: **Fishalike**.

## Requisitos

| | Void Linux | outras |
|---|---|---|
| Go ≥ 1.26 | `xbps-install go` | |
| Qt 5 (headers) | `xbps-install qt5-devel` | `qtbase5-dev` (Debian/Ubuntu), `qt5-base` (Arch) |
| mpv (toca o áudio) | `xbps-install mpv` | |
| yt-dlp (opcional: YouTube/SoundCloud) | `xbps-install yt-dlp` | |

## Compilar e rodar

```sh
go build -o meofishes ./cmd/meofishes   # a 1ª vez demora (compila os bindings do Qt)
./meofishes
```

### Build mais rápido: Qt como biblioteca dinâmica

O `go build` compila os ~337 arquivos C++ do miqt praticamente um por vez.
O `tools/qtlib/qtlib.sh` compila esses arquivos **uma vez, usando todos os
núcleos**, numa biblioteca `libmiqt-qt5.so`, e faz o `go.mod` usar uma cópia
do miqt que só linka contra ela:

```sh
tools/qtlib/qtlib.sh enable      # compila a .so (JOBS=16 OPT=-O1) e ativa
CGO_CXXFLAGS="-O1 -w" go build -ldflags="-s -w" -o meofishes ./cmd/meofishes
tools/qtlib/qtlib.sh status      # o que está ativo
tools/qtlib/qtlib.sh disable     # volta ao build normal
```

O binário passa a precisar da `libmiqt-qt5.so` ao rodar (procura no caminho
de onde foi compilado). Para distribuir, use o build normal.

Ferramentas de linha de comando:

```sh
./meofishes lyrics "The Weeknd" "Blinding Lights"   # testa as letras
./meofishes search ytdlp-search "daft punk"         # testa um plugin de fonte
./meofishes check ./meu-plugin                      # valida um plugin.json
./meofishes css ~/.local/share/meofishes/themes/fishalike
```

Atalhos: `Ctrl+Espaço` tocar/pausar · `Ctrl+←/→` anterior/próxima ·
`Ctrl+F` buscar · `Ctrl+L` letras · `F5` recarregar o tema.

## O que vem junto

Instalados no primeiro uso: tema **Fishalike**, **Arquivos locais** (suas pastas
de música), **YouTube / SoundCloud (yt-dlp)**, **Radio Browser** e o
**Layout: Foco nas letras**.

No catálogo (Plugins/Temas → instalar com um clique): temas **Abyss** e
**Sakura Paper**, **Lavalink**, **LRCLIB**, **ListenBrainz** (scrobble),
**Layout: Cinema** e **SomaFM (plugin em Go)**.

## Provedores de letras

| provedor | precisa de quê | estado |
|---|---|---|
| Lyprio | **URL base + chave de assinatura HMAC** (fornecidas pelos mantenedores do Lyprio) | desligado até configurar |
| BetterLyrics | nada (chave `X-API-Key` só se a API passar a exigir) | ✓ palavra por palavra |
| lrc.red | **User-Agent com nome do app, link e e-mail de contato** + crédito ao lrc.red | ligado depois que você preencher o contato em Ajustes |
| Unison | nada (pede nome e link do app no User-Agent); uso só pessoal/não comercial ou projeto open-source | ✓ (acervo comunitário) |
| Paxsenix | nada para `lyrics.paxsenix.org`; a API nova `api.paxsenix.org` usa chave Bearer | campo opcional de chave |
| Kugou | nada | ✓ palavra por palavra (KRC) |

Configure em **Plugins → Meo Lyrics → Configurar** e em **Ajustes → Contato**.

## Estrutura

```
cmd/meofishes        main + CLI
internal/core        tipos (Track, Lyrics…)
internal/lyrics      parsers TTML/LRC/KRC, provedores e a cadeia de prioridade
internal/plugin      manifesto, gerenciador, runtimes (declarativo/script/processo), módulos Go
internal/builtin     plugins embutidos (Meo Lyrics, Arquivos locais)
internal/theme       compilador de CSS (variáveis, funções de cor, -meo-*), easing
internal/player      mpv via IPC JSON
internal/catalog     catálogo de temas/plugins
internal/ui          interface Qt (miqt)
sdk/                 SDK Go para plugins "process"
assets/              temas, plugins de exemplo e catálogo embutidos no binário
docs/                PLUGINS.md e THEMES.md
```

Guias: [docs/PLUGINS.md](docs/PLUGINS.md) · [docs/THEMES.md](docs/THEMES.md)
