# Temas do meofishes

A interface inteira é desenhada a partir do CSS do tema: cores, fontes,
espaçamentos, bordas e também as animações e os widgets desenhados à mão
(letras, capa, barra de progresso, fundo, ícones).

Na interface, em **Temas**, você pode:

- **Aplicar** um tema instalado com um clique;
- **Baixar e aplicar** temas do catálogo (o embutido e os que você adicionar em
  Ajustes → Catálogos);
- instalar um `.zip`, uma pasta ou uma URL;
- **Criar meu tema**: cria um tema que estende o atual, abre a pasta dele e liga
  o recarregamento automático. É só editar o `style.css` e salvar;
- escrever **CSS do usuário**, aplicado por cima de qualquer tema.

`F5` recarrega o tema. `meofishes css <pasta>` mostra o QSS gerado.

## Estrutura

```
meu-tema/
├── theme.json
├── style.css
└── fonts/Inter.ttf     (opcional)
```

```json
{
  "id": "meu-tema",
  "name": "Meu Tema",
  "version": "1.0.0",
  "author": "você",
  "description": "…",
  "extends": "fishalike",
  "style": "style.css",
  "fonts": ["fonts/Inter.ttf"],
  "preview": { "background": "#241015", "surface": "#3b1a21", "accent": "#d9737a", "highlight": "#f4b4c6", "text": "#fbe9ec" }
}
```

Com `extends`, o CSS do tema pai é carregado antes do seu. Um tema pode ser só
um `:root { … }` com outras cores. O Abyss e o Sakura Paper são feitos assim.

## A linguagem

É o QSS do Qt (praticamente CSS 2 com seletores por classe Qt, `#id` e
`[propriedade="valor"]`) com estes extras:

| recurso | exemplo |
|---|---|
| variáveis | `:root { --accent: #d9737a; }` … `color: var(--accent, #fff);` |
| transparência | `alpha(var(--accent), .4)` |
| mistura | `mix(#ff0000, #0000ff, 0.25)` |
| luz/saturação | `lighten(c, 10%)`, `darken(c, 8%)`, `saturate(c, 20%)`, `desaturate(c, 20%)` |
| importação | `@import "cores.css";` |
| propriedades meofishes | `-meo-alguma-coisa: valor;` (lista abaixo) |

As propriedades `-meo-*` são removidas antes de o CSS ir para o Qt e são lidas
pelos widgets do meofishes. Elas valem para seletores `#Nome`, `Tipo`,
`[role="x"]` e `Tipo[role="x"]`.

Durações: `300ms` ou `0.3s`. Curvas (easing): `linear`, `ease`, `ease-in`,
`ease-out`, `in-out-quad`, `out-cubic`, `in-out-cubic`, `out-quart`,
`out-quint`, `out-expo`, `in-out-expo`, `out-back`, `out-elastic`,
`out-bounce`, `out-sine`, `in-out-sine`.

## Elementos

| seletor | o que é |
|---|---|
| `#Root` | fundo da janela (desenhado pelo meofishes) |
| `#Sidebar`, `#Logo`, `#LogoSub`, `#SidebarFooter` | barra lateral |
| `QPushButton[role="nav"]` (`[active="true"]`) | botões do menu (`#Navhome`, `#Navsearch`, …) |
| `QLabel[role="section"]` | títulos de seção do menu |
| `#Pages` | área das páginas (`#HomePage`, `#SearchPage`, `#QueuePage`, `#LyricsPage`, `#PluginsPage`, `#ThemesPage`, `#SettingsPage`) |
| `QLabel[role="title"]`, `[role="subtitle"]`, `[role="heading"]`, `[role="hint"]` | textos das páginas |
| `#Greeting`, `#SearchBox`, `#SourcePicker`, `#HomeSource`, `#SearchInfo` | início e busca |
| `#TrackRow` (`[playing="true"]`), `#RowCover`, `QLabel[role="row-title"]`, `[role="row-sub"]`, `[role="row-meta"]` | linhas das listas |
| `QFrame[role="card"]` (`[active="true"]`), `[role="card-title"]`, `[role="card-sub"]`, `[role="card-error"]`, `[role="chip"]`, `#Swatch` | cartões de plugins e temas |
| `QPushButton[role="primary"]`, `[role="danger"]` | botões de destaque |
| `#NowPlaying`, `#Cover`, `#NowTitle`, `#NowArtist`, `#NowAlbum` | painel “tocando agora” |
| `#LyricsView` (página), `#MiniLyrics` (painel), `#PanelLyrics` (região de layout) | letras |
| `#LyricsStatus`, `#MiniLyricsStatus` | provedor da letra |
| `#PlayerBar`, `#PlayerCover`, `#BarTitle`, `#BarArtist`, `#TimeElapsed`, `#TimeTotal` | barra do player |
| `QPushButton[role="control"]`, `#Play`, `#Prev`, `#Next`, `#Shuffle`, `#Repeat`, `#LyricsToggle`, `#QueueToggle`, `#VolumeIcon` | botões com ícone |
| `#SeekBar`, `#VolumeBar` | barras deslizantes |
| `#QueuePanel`, `#LyricsPanel`, `#BigCover` | regiões extras dos layouts |
| `#Toast` | avisos |
| `#UserCSS`, `#LayoutPicker` | editor de CSS, seletor de layout |

## Propriedades `-meo-*`

### Fundo (`#Root`)

| propriedade | padrão | |
|---|---|---|
| `-meo-backdrop-from` / `-to` | vinho | gradiente |
| `-meo-backdrop-angle` | `160` | graus |
| `-meo-backdrop-dynamic` | `false` | tinge o fundo com a cor da capa |
| `-meo-backdrop-dynamic-strength` | `0.35` | 0–1 |
| `-meo-backdrop-dynamic-duration` | `1200ms` | transição entre capas |
| `-meo-backdrop-glow` | — | cor de uma luz radial suave |
| `-meo-backdrop-glow-x` / `-y` / `-radius` | `.8` / `.1` / `.8` | posição e tamanho (frações) |
| `-meo-backdrop-glow-pulse` | `0` | período da “respiração” da luz (`0` desliga) |

### Animações de entrada (qualquer região ou `#TrackRow`)

| propriedade | |
|---|---|
| `-meo-enter` | `fade`, `slide-up`, `slide-down`, `slide-left`, `slide-right`, `none` |
| `-meo-enter-duration`, `-meo-enter-delay`, `-meo-enter-easing`, `-meo-enter-distance` | |
| `-meo-enter-stagger` | atraso entre linhas de uma lista (efeito cascata) |

### Troca de página (`#Pages`)

`-meo-transition` (`fade`, `slide-up`, `slide-left`, …, `none`),
`-meo-transition-duration`, `-meo-transition-easing`, `-meo-transition-distance`.

### Botões e linhas com fundo animado

Em `QPushButton[role="nav"]`, `QPushButton[role="control"]`, `#Play`…,
`#TrackRow`:

| propriedade | |
|---|---|
| `-meo-bg`, `-meo-hover-bg`, `-meo-active-bg` | cores; a transição entre elas é animada |
| `-meo-hover-duration`, `-meo-hover-easing` | |
| `-meo-radius` | raio do fundo |
| `-meo-icon-color`, `-meo-hover-icon-color`, `-meo-active-icon-color` | cor do ícone (padrão: `color` do QSS) |
| `-meo-icon-size` | |
| `-meo-press-scale` | “afunda” ao clicar (ex.: `0.9`) |
| `-meo-padding-x` | nav: espaço antes do ícone |
| `-meo-shadow` | `x y blur cor` (sombra real, também em `#Cover`) |

Quando um botão não define `-meo-bg`/`-meo-hover-bg`, vale o `:hover` normal do QSS.

### Capas (`#Cover`, `#PlayerCover`, `#RowCover`, `#BigCover`)

`-meo-radius`, `-meo-crossfade` (duração), `-meo-zoom-on-change` (ex.: `0.94`),
`-meo-zoom-easing`, `-meo-placeholder` (cor sem capa),
`-meo-placeholder-icon`, `-meo-shadow`.

### Barras (`#SeekBar`, `#VolumeBar`)

`-meo-track-color`, `-meo-fill-color`, `-meo-fill-color-2` (gradiente),
`-meo-handle-color`, `-meo-handle-size`, `-meo-handle-on-hover`,
`-meo-height`, `-meo-hover-height`, `-meo-radius`, `-meo-smooth`.

### Letras (`#LyricsView`, `#MiniLyrics`, `#PanelLyrics`)

| propriedade | |
|---|---|
| `-meo-font-family`, `-meo-font-size`, `-meo-font-weight` | |
| `-meo-color` | linhas que ainda não chegaram |
| `-meo-past-color` | linhas já cantadas |
| `-meo-active-color` | parte já cantada da linha ativa |
| `-meo-upcoming-color` | parte ainda não cantada da linha ativa |
| `-meo-word-fill` | `gradient` (borda suave) ou `solid` |
| `-meo-fill-softness` | largura da borda do preenchimento |
| `-meo-glow-color`, `-meo-glow-radius` | brilho nas palavras cantadas (`0` desliga) |
| `-meo-word-lift` | a palavra sobe um pouco ao ser cantada |
| `-meo-active-scale`, `-meo-inactive-scale`, `-meo-scale-duration` | zoom da linha ativa |
| `-meo-scroll-duration`, `-meo-scroll-easing` | rolagem automática |
| `-meo-focus-position` | altura da linha ativa (0 = topo, 1 = base) |
| `-meo-distance-fade` | some aos poucos longe da linha ativa |
| `-meo-align`, `-meo-opposite-align` | `left`/`center`/`right`; o segundo vale para o 2º cantor em duetos |
| `-meo-line-spacing`, `-meo-padding` | |
| `-meo-bg-vocal-color`, `-meo-bg-vocal-scale` | vozes de fundo |
| `-meo-interlude-dots`, `-meo-dot-color` | três pontinhos nos intervalos instrumentais |
| `-meo-line-sync-fill` | letras só por linha se preenchem como uma varredura |
| `-meo-hover-color` | linha sob o mouse (clique para pular até ela) |
| `-meo-credit-color`, `-meo-empty-color` | créditos e mensagens |

### Aviso (`#Toast`)

`-meo-show-duration`, `-meo-hide-after`.

## Exemplo: um tema só de cores

```css
/* theme.json: { "id": "menta", "name": "Menta", "extends": "fishalike" } */
:root {
  --wine-0: #06140f; --wine-1: #0a1d16; --wine-2: #0f271e; --wine-3: #143428; --wine-4: #1b4535;
  --red: #4fd1a5; --pink-light: #b7f5dc; --pink: #7fe6c0; --pink-dark: #2f9e7a;
  --text: #e8fbf3; --text-dim: #a9d6c4; --text-faint: #6f9c8a;
}
#LyricsView { -meo-align: center; -meo-font-size: 40px; }
```
