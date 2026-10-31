package main

// The Learn cards (AltGr + L): the recommended way to write each language
// today, checked against its specification. One card per language. Each
// line says what to use, why (from the spec), and what it is like in the
// real world. Cards for other languages are in learn_*.go.
//
// The box opens on what the cursor is on: every word and symbol in Use
// finds its line, and Keys add more (learn.go).

type learnItem struct {
	Use  string // what to write: a unit, a symbol, a pattern (shown as code)
	Why  string // what it is and when to use it, from the spec
	Like string // the same idea in the real world
	// Example is common real use: 1 to 3 lines of code (joined with \n),
	// each 60 letters or fewer.
	Example string
	Keys    []string // more words or symbols that open this line (lowercase)
}

type learnSection struct {
	Title string
	Keys  []string // words that open this section when no line matches
	Items []learnItem
}

type learnTopic struct {
	Name     string   // shown in the box: "CSS"
	Match    []string // highlighter names and file endings it opens for: "css", ".scss"
	Source   string   // the specifications the card is checked against
	Sections []learnSection
}

var learnCSS = learnTopic{
	Name:   "CSS",
	Match:  []string{"css", "scss", "sass", "less", "stylus", ".css", ".scss", ".sass", ".less", ".styl"},
	Source: "CSS Values and Units 4 · CSS Color 4 and 5 · Selectors 4 · CSS Nesting 1 · CSS Cascade 5 · Media Queries 4 and 5 · CSS Text 4 · CSS Fonts 4 · CSS Box Sizing 3 and 4 · CSS Containment 3 · CSS Grid 1 · CSS Box Alignment 3 · CSS Positioned Layout 3 · CSS Logical Properties 1 · CSS Transforms 1 · MDN Baseline (web-features data)",
	Sections: []learnSection{
		{Title: "Units: which to use", Keys: []string{"font-size", "margin", "padding", "width", "height", "top", "left", "right", "bottom"}, Items: []learnItem{
			{Use: "rem", Why: "Font sizes, spacing and most sizes. 1rem is the font size of the root (<html>) element. Browsers set it to 16px unless the user chose another size.",
				Like: "Like a recipe that says 'two cups'. The cup is set once for the whole kitchen, and every amount grows with it.", Example: "h1 { font-size: 2rem; }\n.card { padding: 1.5rem; }"},
			{Use: "em", Why: "Padding or gaps that should grow with the element's own font size, such as inside buttons. 1em is the element's own font size. Inside font-size itself, it is the parent's font size.",
				Like: "Like a picture frame cut to fit each photo. A bigger photo gets a wider frame.", Example: ".button {\n  padding: 0.5em 1em;\n}"},
			{Use: "px", Why: "Thin borders (1px), shadows and hairlines. Not for font sizes: a px size ignores the user's font-size setting. Page zoom still scales it.",
				Like: "Like a ruler printed on paper. A bigger text setting does not stretch it.", Example: ".card {\n  border: 1px solid #ddd;\n}"},
			{Use: "%", Why: "A size measured against another box. For width, that box is the containing block, which is usually the parent. width: 50% is half of its width.",
				Like: "Like 'half the table'. The size depends on how big the table is.", Example: ".sidebar { width: 25%; }\nimg { max-width: 100%; }"},
			{Use: "fr", Why: "Grid tracks only (CSS Grid). 1fr is one share of the leftover space in the grid container.",
				Like: "Like sharing a pizza: 1fr 2fr means one person gets one slice for every two slices the other gets.", Example: ".layout {\n  display: grid; grid-template-columns: 1fr 3fr;\n}"},
			{Use: "ch", Why: "The advance width of the 0 (zero) glyph in the font. max-width: 65ch keeps lines easy to read. Most letters are narrower than 0, so a line fits more than 65 letters.",
				Like: "Like a newspaper column: narrow enough that the eye finds the next line easily.", Example: "article {\n  max-width: 65ch;\n}"},
			{Use: "dvh · svh · lvh", Keys: []string{"vh"}, Why: "Full-screen heights on phones. 100dvh is the height you can see now. It changes as the browser bar shows and hides. 100svh is the height with the bars shown. 100lvh and 100vh are the height with the bars hidden, so they can be taller than what you see. Baseline 2022.",
				Like: "Like a blind that always matches the glass you can see, even when a shutter covers part of the window.", Example: ".hero {\n  min-height: 100dvh;\n}"},
			{Use: "vw · vi", Why: "1vw is 1% of the viewport width (the large viewport: browser bars hidden). 1vi is 1% of the viewport size in the element's inline axis, the direction text runs. vi: Baseline 2022.",
				Like: "Like measuring a room in percent of the wall, so the plan works in any size room.", Example: ".hero h1 {\n  font-size: calc(1.5rem + 2vw);\n}"},
			{Use: "cqi · cqb", Keys: []string{"container-type"}, Why: "1cqi is 1% of the inline size of the nearest query container. An element becomes one with container-type: inline-size or size. With no container, cqi uses the small viewport size. Baseline 2023.",
				Like: "Like furniture sized to the room it stands in, not to the whole house.", Example: ".cards { container-type: inline-size; }\n.card h2 { font-size: clamp(1rem, 4cqi, 2rem); }"},
			{Use: "unitless line-height", Keys: []string{"line-height"}, Why: "line-height: 1.5 with no unit. Children inherit the number, not a fixed length. Each element multiplies it by its own font size, so nested text keeps good spacing.",
				Like: "Like saying 'one and a half times your height' instead of '90 cm': it fits a child and an adult.", Example: "body {\n  line-height: 1.5;\n}"},
		}},
		{Title: "Sizes that adapt", Items: []learnItem{
			{Use: "clamp(min, ideal, max)", Why: "A size that changes with the screen but stays between two limits. Example: font-size: clamp(1rem, 0.9rem + 0.5vw, 1.25rem). It is the same as max(min, min(ideal, max)). If min is larger than max, min wins.",
				Like: "Like a thermostat set between 19 and 22 degrees: it moves, but never past the limits.", Example: "h1 {\n  font-size: clamp(1.75rem, 1.2rem + 2vw, 3rem);\n}"},
			{Use: "min() · max()", Why: "min() uses the smallest of its values. max() uses the largest. width: min(100%, 60rem) is the full width, but never more than 60rem.",
				Like: "Like 'fill the glass, but no more than 250 ml'.", Example: ".container {\n  width: min(100% - 2rem, 60rem);\n}"},
			{Use: "max-inline-size", Keys: []string{"max-width"}, Why: "The limit along the text direction. In horizontal writing (left-to-right or right-to-left) it is max-width. In vertical writing it is max-height. Baseline 2021.",
				Like: "Like saying 'the length of the line' instead of 'the width', so it is right whichever way you write.", Example: "p {\n  max-inline-size: 65ch;\n}"},
			{Use: "aspect-ratio", Why: "aspect-ratio: 16 / 9 keeps the shape of a box as its width changes. It only works when width or height is auto. Baseline 2021.",
				Like: "Like a TV screen: a bigger TV is still the same shape.", Example: "iframe {\n  width: 100%; aspect-ratio: 16 / 9;\n}"},
			{Use: "box-sizing: border-box", Why: "Set it on *, *::before, *::after. Then width and height include the padding and border, but not the margin.",
				Like: "Like a parcel size that includes the cardboard, not only what is inside.", Example: "*, *::before, *::after {\n  box-sizing: border-box;\n}"},
		}},
		{Title: "Layout", Keys: []string{"display", "grid-template-columns", "grid-template-areas", "justify-content", "align-items", "flex-direction"}, Items: []learnItem{
			{Use: "display: grid", Why: "Layouts in two directions at once: rows and columns. Page areas and card grids.",
				Like: "Like a chessboard or a spreadsheet: everything lines up both across and down.", Example: ".page {\n  display: grid; grid-template-columns: 15rem 1fr;\n}"},
			{Use: "repeat(auto-fit, minmax(16rem, 1fr))", Keys: []string{"auto-fill"}, Why: "A grid with as many columns of at least 16rem as fit. The leftover space is shared between them. No media queries needed. auto-fit collapses empty columns. auto-fill keeps them.",
				Like: "Like setting a table: as many plates as fit along it, each with at least its own space.", Example: ".cards { display: grid; gap: 1rem;\n  grid-template-columns:\n    repeat(auto-fit, minmax(16rem, 1fr)); }"},
			{Use: "display: flex", Why: "Layouts in one direction: a row of buttons, a menu bar, one item in the middle.",
				Like: "Like books on one shelf: they line up in a row and share the shelf.", Example: ".toolbar {\n  display: flex; align-items: center; gap: 0.5rem;\n}"},
			{Use: "gap", Keys: []string{"row-gap", "column-gap"}, Why: "Space between grid or flex items, not at the outer edges. Use it instead of margins on each child. gap in flexbox: Baseline 2021.",
				Like: "Like the grout between tiles: set once, even everywhere, never at the outer edge.", Example: ".nav {\n  display: flex; gap: 1rem;\n}"},
			{Use: "place-items: center", Why: "Short for align-items and justify-items. In a grid, each item is centred in its own grid area in both directions.",
				Like: "Like placing each plate in the exact middle of its own place mat.", Example: ".hero {\n  display: grid; place-items: center;\n}"},
			{Use: "@container", Why: "Changes a component by the space its container gives it, not by the screen size. An ancestor needs container-type. Baseline 2023.",
				Like: "Like a sofa that folds into a chair when the room is small, wherever the room is in the house.", Example: "@container (width > 30rem) {\n  .card { display: flex; }\n}"},
			{Use: "@media (width >= 48rem)", Keys: []string{"@media", "min-width"}, Why: "Range syntax for screen-size breakpoints (Media Queries 4). Baseline 2023. Use rem or em in breakpoints, not px. In media queries they use the browser's default font size, so they follow the user's setting.",
				Like: "Like a rule 'if the table seats 6 or more, add a second dish'.", Example: "@media (width >= 48rem) {\n  .nav { display: flex; }\n}"},
			{Use: "position: sticky", Keys: []string{"position"}, Why: "An element that scrolls with the page, then sticks at an edge of the nearest scrolling area. It needs top (or another side) set to a value other than auto. It never leaves its containing block, usually its parent.",
				Like: "Like a chapter title that stays at the top of the page while you read that chapter, then moves away with it.", Example: "header {\n  position: sticky; top: 0;\n}"},
		}},
		{Title: "Logical properties", Items: []learnItem{
			{Use: "margin-inline · padding-block", Keys: []string{"margin-inline-start", "padding-inline", "margin-block"}, Why: "inline is the direction text runs; block is the direction lines stack. These work in right-to-left and vertical languages. Baseline 2021.",
				Like: "Like saying 'before the text' and 'after the text' instead of 'left' and 'right', so it works in Arabic too.", Example: ".container { margin-inline: auto; }\n.section { padding-block: 2rem; }"},
			{Use: "inset: 0", Keys: []string{"inset"}, Why: "Short for top, right, bottom and left, all at 0. These are physical sides. Values are given in the same order as margin.",
				Like: "Like saying 'touch all four walls' in one sentence.", Example: ".overlay {\n  position: fixed; inset: 0;\n}"},
			{Use: "inline-size · block-size", Why: "inline-size is the size along the direction text runs. block-size is the size in the direction lines stack. In horizontal writing they are width and height.",
				Like: "Like measuring a page by 'line length' and 'page depth'.", Example: ".sidebar { inline-size: 16rem; }\n.banner { block-size: 12rem; }"},
		}},
		{Title: "Colour", Keys: []string{"color", "background", "background-color", "border-color", "fill", "stroke", "#"}, Items: []learnItem{
			{Use: "oklch(70% 0.15 40)", Keys: []string{"oklab", "hsl", "rgb"}, Why: "Lightness (0% to 100%), chroma (how strong; 100% means 0.4), hue (an angle in degrees). Oklab is designed to be perceptually uniform: equal steps in lightness look like equal steps. Baseline 2023.",
				Like: "Like a paint mixer's dials: one for light, one for strength, one for which colour.", Example: "a {\n  color: oklch(55% 0.2 260);\n}"},
			{Use: "color-mix(in oklch, …)", Why: "Mixes two colours in a chosen colour space. color-mix(in oklch, var(--brand), white 20%) is 80% brand and 20% white. Baseline 2023.",
				Like: "Like mixing two tins of paint in a set ratio.", Example: ".button:hover {\n  background: color-mix(in oklch, var(--brand), black 15%);\n}"},
			{Use: "rgb(0 0 0 / 50%)", Keys: []string{"rgba"}, Why: "The modern syntax: spaces between values, and / before the alpha (see-through amount). rgba() is an old alias of rgb(), so it is not needed.",
				Like: "Like tinted glass: the colour, then how much you can see through it.", Example: ".backdrop {\n  background: rgb(0 0 0 / 50%);\n}"},
			{Use: "currentColor", Keys: []string{"currentcolor"}, Why: "The value of the element's own color property (its text colour). Borders and icons then follow the text.",
				Like: "Like a pen whose ink is used for both the writing and the underline.", Example: ".icon {\n  fill: currentColor;\n}"},
			{Use: "light-dark(#fff, #111)", Keys: []string{"color-scheme"}, Why: "Gives the first colour when the element's colour scheme is light and the second when it is dark. Needs color-scheme: light dark. Baseline 2024.",
				Like: "Like a coat with two sides: one for sun, one for night.", Example: ":root { color-scheme: light dark; }\nbody { background: light-dark(#fff, #111); }"},
			{Use: "--brand: oklch(…)", Keys: []string{"var", "--"}, Why: "Custom properties for colours you reuse. Read them with var(--brand).",
				Like: "Like a paint chart with names: change 'brand orange' once and every wall painted with it changes.", Example: ":root { --brand: oklch(60% 0.18 250); }\n.button { background: var(--brand); }"},
		}},
		{Title: "Selectors", Items: []learnItem{
			{Use: ".class", Why: "Style with classes. One #id beats any number of classes, so it is hard to override. Bare tag names reach too far.",
				Like: "Like a name badge: anyone wearing it gets the same treatment.", Example: ".button { padding: 0.5em 1em; }\n.button-primary { background: var(--brand); }"},
			{Use: ":where(…)", Why: "Groups selectors with zero specificity (strength), so later rules override it easily. Good for base styles. Baseline 2021.",
				Like: "Like a pencil sketch: anything drawn over it wins.", Example: ":where(ul, ol) {\n  padding-inline-start: 1.5rem;\n}"},
			{Use: ":is(…)", Why: "Groups selectors. It takes the specificity of its most specific selector. Baseline 2021.",
				Like: "Like a group ticket priced at the most expensive seat in the group.", Example: ":is(h1, h2, h3) {\n  line-height: 1.2;\n}"},
			{Use: ":has(…)", Why: "Selects an element by what it contains: .card:has(img). Baseline 2023.",
				Like: "Like labelling a box 'has glass inside' so it is handled with care.", Example: ".card:has(img) {\n  padding-block-start: 0;\n}"},
			{Use: ":focus-visible", Keys: []string{":focus", "outline"}, Why: "Matches a focused element only when the browser decides a focus ring helps, such as keyboard use. Baseline 2022. Never remove outlines without another visible focus style.",
				Like: "Like a torch that lights the step you are on, so you never lose your place.", Example: ":focus-visible {\n  outline: 2px solid currentColor; outline-offset: 2px;\n}"},
			{Use: "&", Why: "Native CSS nesting: .card { & h2 { … } }. & stands for the outer rule's selector. Baseline 2023, no build tool needed.",
				Like: "Like sub-folders inside a folder: the inner rules belong to the outer one.", Example: ".card {\n  &:hover { background: #f5f5f5; }\n}"},
			{Use: "@layer", Why: "Puts rules into ordered layers (reset, base, components). For normal rules a later layer wins, whatever the specificity. Rules outside any layer beat all layers. For !important rules the order is reversed. Baseline 2022.",
				Like: "Like layers of clothing: the outer layer is what people see, however thick the inner one is.", Example: "@layer reset, base, components;\n@layer components { .button { padding: 0.5em; } }"},
		}},
		{Title: "Text", Keys: []string{"font", "font-weight", "letter-spacing", "text-align"}, Items: []learnItem{
			{Use: "font-family: system-ui, sans-serif", Keys: []string{"font-family"}, Why: "system-ui is the platform's own interface font: fast and familiar. End the list with a generic family such as sans-serif.",
				Like: "Like wearing your everyday shoes: they fit and need no breaking in.", Example: "body {\n  font-family: system-ui, sans-serif;\n}"},
			{Use: "font-display: swap", Keys: []string{"@font-face"}, Why: "Inside @font-face. Text is hidden for at most a very short time (100ms or less is recommended). Then it shows in a fallback font and swaps when the web font arrives.",
				Like: "Like reading a photocopy while the printed book is on its way.", Example: "@font-face { font-family: \"Inter\";\n  src: url(\"/fonts/inter.woff2\") format(\"woff2\");\n  font-display: swap; }"},
			{Use: "text-wrap: balance", Why: "Evens out the line lengths of short text such as headings. Browsers may skip it when there are more than ten lines. Baseline 2024.",
				Like: "Like stacking plates so each pile is the same height.", Example: "h1, h2, h3 {\n  text-wrap: balance;\n}"},
			{Use: "text-wrap: pretty", Why: "Asks for better line breaks at some cost in speed. Browsers use it, for example, to avoid a very short last line. Not Baseline: Chrome 117+ and Safari 26+, not Firefox. Other browsers ignore it safely.",
				Like: "Like a typesetter who rereads the whole paragraph before choosing where the lines break.", Example: "p {\n  text-wrap: pretty;\n}"},
		}},
		{Title: "Motion and access", Keys: []string{"animation", "@keyframes"}, Items: []learnItem{
			{Use: "@media (prefers-reduced-motion: reduce)", Why: "Matches when the user asked their device for less motion (Media Queries 5). Turn off or calm big animations inside it.",
				Like: "Like a lift with a gentle setting for people who get dizzy.", Example: "@media (prefers-reduced-motion: reduce) {\n  * { animation: none; transition: none; }\n}"},
			{Use: "transition: opacity 200ms ease", Keys: []string{"transition"}, Why: "Name the property that moves. transition: all also animates things you did not mean to.",
				Like: "Like telling a mover which boxes to carry, not 'everything in the house'.", Example: ".button {\n  transition: background-color 200ms ease;\n}"},
			{Use: "transform · opacity", Keys: []string{"translate", "scale", "rotate"}, Why: "Animate these. A transform does not change the layout of the content around it, so the browser can skip layout. Animating width, top or margin redoes layout every frame.",
				Like: "Like sliding a picture along a wall instead of rebuilding the wall each time.", Example: ".menu { transform: translateY(-100%); opacity: 0;\n  transition: transform 200ms, opacity 200ms; }\n.menu.open { transform: none; opacity: 1; }"},
			{Use: "prefers-color-scheme", Why: "A media query for the user's light or dark setting (Media Queries 5).",
				Like: "Like a house that turns its lights down when it gets dark outside.", Example: "@media (prefers-color-scheme: dark) {\n  :root { --bg: #111; --text: #eee; }\n}"},
		}},
		{Title: "Avoid", Items: []learnItem{
			{Use: "!important", Why: "Only to override CSS you cannot change. Otherwise fix the selector or use @layer.",
				Like: "Like shouting in a meeting: it works once, then everyone shouts.", Example: "/* instead: */\n@layer base, components;\n@layer components { .button { color: white; } }"},
			{Use: "float for layout", Keys: []string{"float"}, Why: "Use grid or flex for layout. float is for text that flows around an image.",
				Like: "Like using tape to hold up a shelf: brackets were made for the job.", Example: "/* instead: */\n.layout { display: flex; gap: 1rem; }"},
			{Use: "100vh on phones", Why: "Use 100dvh. 100vh is the height with the browser bars hidden. It is taller than the visible screen while a bar shows.",
				Like: "Like a curtain cut for the window frame, not the glass: the bottom ends up hidden.", Example: "/* instead: */\n.hero { min-height: 100dvh; }"},
			{Use: "px font sizes", Why: "Use rem, so text grows when the user sets a bigger font.",
				Like: "Like printing a menu in tiny letters and taking away the reading glasses.", Example: "/* instead: */\nbody { font-size: 1rem; }\nh1 { font-size: 2.5rem; }"},
			{Use: "vendor prefixes by hand", Keys: []string{"-webkit-", "-moz-"}, Why: "Let a build tool (Autoprefixer, Lightning CSS) add them only where your browser list needs them.",
				Like: "Like writing every address in five formats in case one post office is old.", Example: "# instead:\nnpx postcss style.css --use autoprefixer -o out.css"},
		}},
	},
}
