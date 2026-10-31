# Feature preview generation

Generated with the built-in image_gen tool. Final PNGs are documentation illustrations, not Windows runtime screenshots. Example game/device data is fictional. UI previews use renders of the actual built frontend; folder/navigation/handoff diagrams and the settings montage explain the feature rather than adding app controls. The main preview uses frontend/src/assets/images/background-space.png as its authoritative wallpaper input.

## stagebox-preview

Output: docs/images/stagebox-preview.png

Use case: compositing. Edit image 1, the existing StageBox fullscreen library preview. Image 2 is the actual repository wallpaper background-space.png and is the authoritative background insert. Replace only the wallpaper layer behind the interface with image 2, using the same centered cover scaling as the built application. Preserve the exact supplied starfield, emerald horizon and perspective grid, including the actual white widely spaced STAGEBOX title and turquoise PLAY STARTS HERE subtitle at their original positions. These letters may naturally be occluded by the second row of cards, but do not erase or relocate the lettering. Keep all ten cover images, game-card text, badges, dimensions, row/column positions, selected first-card border, upper toolbar and all icons unchanged. In particular retain the proper white gear in the rightmost upper-right Tools button, with teeth and central hole; never substitute a square. Preserve "8:42 PM", "65%", connected controller count "2", RetroBat/Steam icons and the dark translucent toolbar. No sidebars, footer, taskbar, device frame, added caption or invented functionality. Crisp flat front-view desktop UI illustration, landscape 16:9, no blur. The result remains a labeled illustrative preview with fictional games.

## controllers

Output: docs/images/feature-controllers.png

Reference inputs: `/tmp/stagebox-reference-controllers.png`

Use case: ui-mockup. Create a faithful StageBox documentation feature preview from the attached REAL built-interface reference screenshot. Topic: controller setup, mapping and diagnostics. Keep the existing centered dark emerald controller-test panel, the exact generic controller outline with unlettered face buttons, sticks, D-pad, triggers, shoulder buttons, Home, Back/Start, and the existing arrangement of controls and footer. Use the title "Living-room controller · test" (omit the example ID), the green "Move a control" readout, "Release controls", disabled "Test rumble", "Back to settings", collapsed "Input details", "Hold Select + Start (1s) to release." and the bottom-right X + "Close" button. Preserve text hierarchy, actual blank face-button positions and native rounded-rectangle dialog styling. Give the South face button and left stick a small cyan/emerald diagnostic highlight to communicate input testing; do not invent extra controller actions. Keep the dimmed/blurred library behind the panel with the emerald grid wallpaper. Crisp front-view 16:9 UI illustration, legible large panel, no external title, no annotations, no device frame, no sidebars. This is a labeled illustrative documentation preview with example data, not a hardware screenshot.

## virtual-input

Output: docs/images/feature-virtual-input.png

Reference inputs: `/tmp/stagebox-reference-virtual-input.png`

Use case: ui-mockup. Create a faithful StageBox documentation preview of optional Virtual Xbox input, based on the attached actual built-interface screenshot. Preserve the exact centered Game Options panel and real toggle: small emerald eyebrow "GAME OPTIONS", game title "Canyon Rally", bordered green toggle row titled "Virtual Xbox input for this game", explanatory copy "Also convert controllers saved as Native. Controllers saved as Virtual already use Xbox input.", green pill "On", X + "Close" button, and bottom "A · select   B · back" hint. Retain the dark translucent emerald styling, spacing and proper white close X. Behind it use the dimmed blurred StageBox library and emerald wallpaper. Tighten the framing slightly so the actual toggle panel reads clearly as a thumbnail, while retaining generous surrounding background. Do not add controller shopping art, download buttons, game installation buttons or nonexistent menus. Flat front-view crisp 16:9 desktop UI illustration, not a Windows screenshot. No new headings outside the panel and no watermark.

## steam

Output: docs/images/feature-steam.png

Reference inputs: `/tmp/stagebox-reference-steam.png`, `/home/raven/DEVELOP/launcher/docs/images/stagebox-preview.png`

Use case: ui-mockup. Make a faithful StageBox Steam/update documentation preview. Image 1 is the actual Steam-update dialog rendered from project code and is authoritative for UI layout. Image 2 supplies the polished fictional library covers and repository wallpaper beneath the dim overlay. Keep the actual centered landscape dialog: small dot + "STEAM UPDATE", large title "Aether Drift", message "An update is needed before you play.", three buttons labeled exactly "Open in Steam", "Update in Steam", "Back to launcher", and small hint "Choose an action · A / Enter · B / Esc to return". First button has the emerald focus ring. Use the blue space-flight art from Aether Drift as very dark subdued artwork inside the dialog, retaining strong text contrast. Behind it keep the dimmed StageBox library with its real Steam folded-corner card badges. No download percentages, no invented Install/Buy buttons, no Steam account data, no extra menus. Crisp 16:9 front-view desktop UI illustration, actual hierarchy and spacing preserved, no external heading or watermark. The sample game is fictional and the image will be labeled illustrative.

## settings

Output: docs/images/feature-settings.png

Reference inputs: `/tmp/stagebox-reference-settings.png`, `/tmp/stagebox-reference-bluetooth.png`, `/tmp/stagebox-reference-wifi.png`

Use case: ui-mockup. Create ONE 16:9 landscape documentation montage showing three StageBox settings features. Images 1, 2 and 3 are actual built-interface dialog references: Sound Settings, Bluetooth devices, Wi-Fi Networks. Make three evenly spaced cropped dialog previews side by side on one dark emerald backdrop, using the actual visual language and layouts from those images. This is an editorial montage of separate screens, not a new combined application dashboard. Left crop: "Sound Settings", volume slider at "65%", selected "TV speakers" row and "Wireless headset" row, proper small speaker glyph instead of the screenshot's missing-font square, Refresh and X Close controls. Middle crop: "Bluetooth devices", "Pair device" button, "Living-room controller" with green "Connected" badge, "Wireless headset" with "Paired" badge, real Bluetooth icon. Right crop: "Wi-Fi Networks", rows "Living room" and "Guest network" with "96% Saved" and "68% Secured", Refresh and X Close. Crop or scale each real dialog consistently so all three fit with generous margins and no overlap; some secondary fine print may be omitted, but do not invent controls or functionality. Maintain off-white readable labels, dark panels, subtle borders and emerald selection highlights. No devices, desktop taskbar, poster slogan, fake Add Network controls or account information. No watermark. The three separate feature previews should read clearly at thumbnail size.

## first-session

Output: docs/images/feature-first-session.png

Reference inputs: `/home/raven/DEVELOP/launcher/docs/images/stagebox-preview.png`

Use case: infographic-diagram. Create a polished 16:9 StageBox documentation preview explaining basic controller navigation. Input image is the actual-style StageBox library preview and should supply the exact game-card/toolbar language and emerald wallpaper. Composition: a large, faithful cropped view of the library occupying the upper two-thirds, with the first Aether Drift card visibly selected by its actual thin emerald outline. Below that view, in a clearly separated documentation legend (not a new application footer), show a simple neutral generic gamepad and three short readable labels with matching small control symbols: "D-pad / stick · Browse", "A · Play", "B · Back". Use the same near-black emerald background, off-white text and restrained green accents. Keep the main interface toolbar's proper white gear at upper right and no missing-font squares. The library view contains fictional example covers as in the reference. Do not add new menu buttons, a sidebar, player avatars, software features or a desktop taskbar. Make it clear visually that the controller legend is explanatory artwork outside the app, not shipped interface. Crisp friendly front-view instructional illustration, no watermark, no long paragraphs.

## game-library

Output: docs/images/feature-game-library.png

Reference inputs: `/home/raven/DEVELOP/launcher/docs/images/stagebox-preview.png`

Use case: infographic-diagram. Make a clear 16:9 documentation illustration explaining how StageBox adds a local game. Use the supplied StageBox preview as visual language and the "Canyon Rally" fictional racing cover as the card reference. On the left, show one clean Windows-style folder diagram labeled "C:\games\Canyon Rally", with exactly three visible file rows: "lc.json", "Game.exe", "cover.jpg". The lc.json row can have a small braces-file symbol. On the right, show the actual-style Canyon Rally landscape game card with its desert racing art, title in a dark lower strip and a small keyboard/controller support icon. Draw one restrained emerald arrow from the folder to the card, showing that a local launch file puts the game into the library. Small explanatory caption: "Your game folder → your StageBox library". This is documentation artwork, NOT an app screenshot: do not invent an in-app file browser, Add Game button, upload dialog, importer wizard or software controls that do not exist. No Steam folded-corner badge on this local card. Use near-black emerald atmosphere and a hint of the repository's perspective-grid wallpaper; generous clear spacing, crisp legible labels, friendly practical design, no watermark.

## retrobat

Output: docs/images/feature-retrobat.png

Reference inputs: `/home/raven/DEVELOP/launcher/docs/images/stagebox-preview.png`

Use case: infographic-diagram. Create one friendly 16:9 StageBox feature illustration explaining the RetroBat handoff. Use the supplied StageBox library preview as the exact source for its dark emerald palette, game-card styling and RetroBat shortcut logo (the white arcade-controller/bat silhouette at the top-right, to the LEFT of the Steam button). Composition: three clear steps across the image. Left: a small faithful StageBox library view labeled "StageBox". Center: a larger separate RetroBat destination tile with that white RetroBat emblem, the word "RetroBat" and a small tasteful abstract 8-bit arcade motif; do NOT invent an actual RetroBat menu screenshot, specific emulator controls, commercial game characters, or ROM cards inside StageBox. Right: a small StageBox library view again labeled "Back to StageBox". Connect left to center and center to right with restrained emerald arrows. Beneath the first arrow label "Open RetroBat"; beneath the second label "Quit RetroBat". Main caption at bottom: "Your retro collection, one button away". Keep the source library's proper gear icon and Steam/RetroBat button order. The separate destination tile must clearly represent another frontend, not a new StageBox library section. Flat front-view documentation artwork, clear large type, original symbolic pixel art only, background based on the supplied emerald space/grid artwork. No added app functionality, no watermarks.

## steam

Output: docs/images/feature-steam.png

Reference inputs: `/home/raven/DEVELOP/launcher/docs/images/feature-steam.png`

Use case: precise-object-edit. Edit the attached StageBox Steam feature illustration. Correct ONLY the small missing-glyph square in the rightmost Tools button at the top-right of the background toolbar: replace it with a proper white gear/cog with visible teeth and a center hole, matching the neighboring icon weight and button size. Retain the dimmed backdrop treatment. Do not change the Steam update dialog, titles, buttons, Aether Drift cover, fictional library, spacing, colors, any other icon, or any other content. The result must otherwise match the supplied illustration exactly.
