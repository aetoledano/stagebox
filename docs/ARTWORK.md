# Launcher artwork

Only artwork referenced by the launcher source or its build is retained in the
current tree. Unused alternatives, raster references, design previews, duplicate
loading artwork and the unused Nunito font were removed on October 30, 2026.
Earlier versions remain available in Git history; this cleanup does not rewrite it.

The project owner reports that most supplied images/icons were AI generated
and some icons were downloaded from free sites. That is not a per-file source
or license record. Known generation records are retained below; source URLs and
license terms for other supplied icons have not been recorded.

| Asset | Used by | Recorded origin |
| --- | --- | --- |
| `assets/app-icon.png` | Makefile `app-icon` target / Windows executable | Supplied StageBox app icon; exact origin not recorded |
| `assets/loading-space.png` | `main.go` embed / native transition wallpaper | Generated with the built-in imagegen tool; Space Launch loading prompt below |
| `frontend/src/assets/images/background-space.png` | `frontend/src/css/base.css` | AI-generated subtitle variant of the loading wallpaper |
| `frontend/src/assets/images/controller-ico.png` | Controller-count mask in `css/topbar.css` | Supplied icon; exact source not recorded |
| `frontend/src/assets/images/controller-ico-2.png` | Game-card controller support in `main.js` | Supplied icon; exact source not recorded |
| `frontend/src/assets/images/kbm-ico.png` | Game-card keyboard/mouse support in `main.js` | Supplied icon; exact source not recorded |
| `frontend/src/assets/images/controller-test.svg` | Controller test and mapping diagram imports | Project vector diagram |
| `frontend/src/assets/images/close-ico.svg` | Shared Close button mask in `css/dialogs.css` | Project vector Close control |
| `frontend/src/assets/images/update-ico.svg` | Steam card update indicator in `main.js` | Project vector update control |
| `frontend/src/assets/images/steam-ico.svg` | Topbar Steam button and Steam cards | Simple Icons, CC0; attribution in THIRD_PARTY_NOTICES.md and SVG comment |
| `frontend/src/assets/images/retrobat-ico.svg` | Topbar RetroBat button | Project vector trace of a supplied RetroBat logo JPEG; unused JPEG removed |
| `frontend/src/assets/images/xinput-wordmark.png` | Native-game virtual-input badge in `main.js` | Built-in imagegen extraction from a supplied emblem; prompt below |

Keep these paths synchronized with code imports, CSS URLs, HTML references,
Go embed directives and Makefile inputs. The frontend uses system fonts; no
custom font is bundled. Preserve brand identification notices separately from
StageBox's code license.

## Space Launch loading wallpaper generation prompt

Use case: style-transfer.
Asset type: a polished full-screen 16:9 loading fallback wallpaper for the StageBox game launcher.
Input image 1 is the edit target and source for the bottom launch platform/perspective floor. Preserve that floor's low camera angle, receding grid and luminous front edge. Remove the enormous central moon/planet completely.
Primary request: combine that sci-fi launch platform with the approved Horizon design: dark almost-black emerald atmosphere, a fine bright green horizon, clean widely spaced white STAGEBOX typography. Above the platform show deep open space with many delicate scattered stars, conveying launching into space. The result should feel cohesive with a premium dark green game launcher.
Composition: 16:9 landscape, straight horizon around 75% height. Perspective gridded platform fills bottom quarter, extends edge to edge, and vanishes toward the middle of the horizon. Subtle emerald haze at the vanishing point. Top three quarters are a spacious star field, not a planet. Center the main typography around 42% height with generous negative space. Keep critical text within central 70% for cover-cropping safety. No surrounding application frame or UI.
Lighting/color: near-black blue-green sky, emerald and mint platform lines and soft ambient glow, restrained cool teal depth. Recolor the old saturated hot pink glow to emerald; retain the cinematic perspective and platform architecture. Bright accents should stay confined to the horizon/platform, with readable light typography.
Text verbatim: "STAGEBOX" in a crisp elegant sans serif, all capitals, medium weight, generously tracked letters, matching the calm Horizon title. Beneath it place "LOADING…" in smaller all-capitals, spaced light mint letters. These are the only two text strings. No fake percentage or progress bar.
Avoid: any moon, planet, spacecraft, rocket, people, buildings, trees, mountains, slogans, logos, watermark, panels, chrome, cursors, intense pink, busy HUD graphics, illegible lettering.
Deliver a high-resolution clean 16:9 wallpaper. This is a new sibling design; do not alter the original reference file.

## Space Launch library wallpaper generation prompt

Use case: text-localization.
Input image 1 is the edit target: the approved StageBox Space Launch wallpaper.
Make exactly one change: replace the small subtitle "LOADING..." beneath STAGEBOX with the exact phrase "PLAY STARTS HERE". Use the same mint color, elegant widely spaced uppercase sans-serif style and approximate letter size, centered under STAGEBOX. No ellipsis on the new phrase.
Preserve everything else as faithfully as possible: exact STAGEBOX title style and position, emerald platform grid, luminous front edge, straight horizon at the same height, stars and nebula pattern, colors, lighting, contrast, perspective, dimensions and 16:9 framing. Do not redraw/recompose the scene. Do not add a moon, planet, rocket, extra objects, watermark, UI frame, progress indicator or any other text. This variant will be the wallpaper behind the game cards, matching the unchanged LOADING version used as the fallback.

## XInput wordmark generation prompt

Use case: background-extraction. Edit the supplied XInput circular emblem. Keep ONLY the exact 'XInput' wordmark from the bottom: the wide stylized X and the geometric lettering I-n-p-u-t. Preserve that lettering's shapes, proportions, relative heights and horizontal arrangement. Remove the circle, rings, patterns and all four face-button illustrations. Recolor the wordmark a single flat StageBox emerald green #22AA77. No cyan, no neon bloom, no bevels or colored background. Produce clean opaque lettering with crisp antialiased edges on genuine transparent alpha. Reframe it as a tightly cropped wide horizontal logo, approximately 4:1 aspect ratio, with only a small even transparent margin. The lettering should fill almost the entire canvas. Exact text: 'XInput'. This is a compact game-card indicator.
