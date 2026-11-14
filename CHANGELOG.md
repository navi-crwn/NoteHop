# Changelog

What changed in NoteHop, newest first, written for the people who use it.

## [Unreleased]

## [1.4.0] - 2026-11-14 - Reset vault and a tidier dashboard

A way out for people who lost their vault key, and a calmer signed-in area.

### Added

- **Security page.** A HOP ID account that forgot its vault key and has no recovery codes can now start again with "Reset vault". It deletes everything the old key protected (encrypted notes with their attachments and share links, margin notes and recovery codes), keeps your basic notes, signs the other devices out and lets you set a new vault key straight away. It asks in two steps, first showing how much will be lost and then asking you to type RESET, because there is no way back, and it only runs for a sign-in that HOP ID confirmed in the last few minutes. NoteHop never held the key, so giving up the old encrypted notes is the only way to a new one, and people who lost the key were stuck before.
- **My notes.** The home of the dashboard now shows your basic notes straight away, without asking for the vault key, and a Vault button (also in the sidebar) leads to the encrypted ones. The ten filters became the five that mean something for a list of basic notes. Most people care about their basic notes first, and they had to unlock the vault just to see them. A new note started from here begins as a basic note.
- **My notes, Edit.** A basic note whose owner link is not on this device now has an Edit button. After you confirm, it gives the note new editing keys, so you can edit it without the vault. Older owner links and edit links you gave to other people stop working, because they depend on the old keys; reading links keep working. Before, losing the vault meant losing the ability to edit your own basic notes.
- **Colours.** Two softer colour palettes, Mist and Sand, can be chosen from the language menu next to the classic one, which stays the default for now. Many people found the bright green to purple buttons too loud.
- **Editor.** Inside the dashboard every note editor has a full screen button that hides the sidebar and the top bar, and Escape leaves it. Writing needs more room than the dashboard frame leaves.
- **Web notes.** The divider between the code and the result can be dragged (or moved with the arrow keys) to make the result bigger or smaller, and the list of files can be shown as a row above the code instead of beside it. Both choices are remembered in your browser and do not apply on phones, where the panes are stacked.
- **Dashboard home.** A card at the top can show a notice from the people who run the site, such as planned maintenance. Close it and it stays away for a week; when there is no notice there is no card. Every dashboard in the family now has a place for announcements.

### Changed

- **New note page.** Code and Web are no longer in the "start from" row. They are formats in the editor's own Format menu, next to Plain and Document, and choosing one lets go of the starter that was lit. The same choice used to sit in two places that could disagree.
- **Nearby devices, Compare texts, How it works and FAQ.** Signed-in people now open these inside the dashboard, with the sidebar in place, instead of landing on a public page with a different top bar.
- **Back links and the logo.** When you are signed in, "Back to NoteHop", the logo and the way out of an error page lead to your dashboard instead of the public home page. Visitors without an account still go to the home page.
- **Sidebar.** "Part of Hop Easy Link" now sits at the bottom left of the sidebar, as in the other HOP apps, and is no longer repeated in the page footer.

### Fixed

- **Main buttons.** The gradient of the main buttons is laid out once over the whole button, and its violet end is a little deeper in the dark theme, so the right end no longer reads as a bright block. The cause was a gradient that was laid out on the inside of the button and repeated under its border.
- **Reset vault on other devices.** A device that still held the old vault key after a reset used to sign back in and quietly mix notes sealed with the old key and the new one. The vault now has a generation number, and a key from an older generation is dropped at once, so you are asked for the new key. Also, a reset no longer works from a sign-in that HOP ID has not confirmed recently, even when HOP ID cannot be reached.
- **Top bar on small screens.** On phones the buttons of the public top bar wrapped onto a second line and covered the page below. They now stay in one row; the help menu moves into the menu behind the burger button on the narrowest phones, and nothing overlaps in any of the nine languages.
- **Nearby devices.** The pairing code box showed only five of the six digits. It is now wide enough for all six on every screen.

## [1.3.1] - 2026-11-12 - Search engines, buttons and a shared look

### Fixed

- **Main buttons.** The main buttons no longer show a bright line on their right edge. The colour gradient was repeating underneath the border.
- **Web notes.** The result box now says what it blocked, such as pictures, scripts or fonts the code asked for from the internet, so a note that looks empty explains why. The box itself stays sealed. Blocked requests used to fail without a word.
- **Language choice.** The language you pick is saved once per visit instead of twice. Nothing changes on screen.
- **Search engines.** Only the public pages (home, new note, the info and tool pages) can appear in search results. Notes, links, accounts and every other page tell search engines to stay away, so private content is never listed.

### Changed

- **Light or dark theme and language across HOP apps.** The theme and language you choose are now shared between the HOP apps, so you set them once and every app follows. Copies installed on your own server are not affected.

## [1.3.0] - 2026-11-12 - Command line, attachments and quick presets

### Added

- **Command line.** The notehop get command can read the note link from standard input, so it can follow notehop new in one line.
- **Command line.** A new decrypt command opens a saved note file offline, without contacting any server.
- **Downloads.** Programs for Linux, macOS and Windows are now built and attached to each release together with their checksums, so people can install without building them.
- **Secret message and Time capsule.** Two addresses open the new-note form with the setting already chosen: a message that can be read once and is gone after a day, and a note that opens on a date you pick. Both are in all nine languages and linked from the home page tour.
- **Attachments.** The person who runs a server can now choose how long attachments are kept and how much one signed-in account may hold (100 MB by default). Over the limit an upload is refused with a clear message in nine languages, and the editor shows how much of your share is used. This stops one account from filling the disk.

### Changed

- **Attachment notices.** The FAQ and the "attached" and "expired" messages no longer promise exactly 24 hours, because the person who runs the server can change it.

## [1.2.0] - 2026-11-11 - Account switching, a guest limit and PayHop

### Added

- **Creating notes without an account.** One address can create a limited number of notes per day without signing in (200 by default). People who are signed in are never counted, and over the limit the message is shown in nine languages. This keeps the service usable when someone floods it.
- **HOP family.** PayHop joins the family list, the home page tiles, the README tables in all nine languages and the documentation site. The shared fonts, tiles and app marks, including BioHop's new mark, come from a newer shared version.

### Fixed

- **Switching HOP ID account.** Switching to another account at the hub no longer leaves the previous account signed in here. The first page you open afterwards quietly checks with the hub: the same account carries on, another account replaces the old one, and with no hub session you are signed out of the dashboard while public pages stay open. If the hub cannot answer, nobody is signed out or sent away.

### Changed

- **Link names.** The names secret, embargo and send are reserved for the site.
- **Writing help.** The translate menu no longer offers Arabic, which the site does not support.
- **Search engines.** The sitemap lists only the public pages, never a note, profile or link name. Notes made over SSH count toward the guest limit too.
- **Home page.** The family tiles stay in one row of five down to narrow windows, the main button reads "Try a demo", and the repeated feature pictures are gone.

## [1.1.0] - 2026-11-10 - A clearer dashboard and shared dialogs

### Added

- **Footer.** Every footer starts with the HOP ID mark and "Part of Hop Easy Link", linking to the hub.
- **My notes.** A reminder card appears while the account has no recovery codes, explaining what is at stake and offering a button to make them. You can hide it for a week, and it disappears for good once the codes exist.
- **HOP family.** FileHop, the sixth HOP app, appears in the dashboard sidebar, the app switcher, the footer and the README and documentation tables.
- **Note picker for other apps.** BioHop can show a signed-in member's public notes so they can be chosen from there. Encrypted notes, notes with a content warning and notes meant to burn after reading are never offered.

### Changed

- **Every page.** All buttons, fields and menus follow the shared HOP control standard: one corner shape, three heights, at least 44 pixels on touch screens and readable text in fields on phones. Buttons are as wide as their text and no longer stretch across dialogs.
- **Questions and confirmations.** Deleting or confirming now uses the HOP family's shared dialog, with a red button that names what will be deleted, Cancel selected first, Escape to cancel and the focus returned to where you were. If the shared dialog cannot load, the browser's own dialog is used instead.
- **New note page.** One editor and one row of settings, each reading "name: value": type, password, delete, opens and readers. The password and the view limit have their own settings, a setting that cannot be used explains why, and the Create button sits next to a sentence saying what the link will do. If you are signed in with the vault locked, the page says so before the note is made and lets you unlock without losing the draft.
- **Signed-in pages.** New note and Security use the same frame as My notes, with the sidebar and the HOP family. The public top bar keeps New note, My notes, Compare texts and Nearby devices, and puts How it works and the FAQ behind one help button. Working pages end in a one-line footer.
- **Header and footer.** The app name is real text in the app's own typeface, and the family marks are updated.
- **Home page.** The "everyday uses" boxes and feature screens use the shared grid, so every row is full.
- **Reports.** A report pasted with a note key in an unusual form now loses the key too, including reports already stored.
- **Self-hosting.** The sample settings file lists every setting, a test keeps it complete, and the dry run of the installer no longer creates folders.

### Fixed

- **My notes.** The list no longer shows a dead "Locked" row for every note the vault cannot open. A basic note is listed by its name with a working link in every state, and an encrypted one says why its title is hidden and how to fix it: unlock, enter the key again, or save the keys from the device that made the note. The page says whether the vault is open and lets you lock it again without signing out. Search, to-dos, links, code, the graph and export work for basic notes without the vault and say how many locked notes they left out. Names of notes remembered on this device are grouped separately, and "account key" is now called "vault key" in all nine languages.
- **Decoy password.** A decoy password now looks exactly like a stranger's view, even for the owner. It used to give itself away, which is the one case where it must not.
- **Dialogs and menus.** Menus and drawers close with Escape and give the focus back, and the Share dialog's tabs fade at the edge on a phone to show that they scroll.
- **Sign-in form.** The local sign-in form says it needs a NoteHop username and not an email address, and points to "Continue with HOP" when an email is typed.
- **Wording.** Small copy fixes, such as "Enter the password" and counts that read correctly for one.
- **Family tiles and sign-out.** The home page tiles all assemble, the footer no longer repeats the language list, and signing out of a HOP ID account returns you to NoteHop.
- **Changelog.** The changelog page lists every merged change again.

### Security

- **Note picker.** The note picker used for BioHop now has a secret of its own, so a leak on this server can no longer forge a sign-in for BioHop.

## [1.0.0] - 2026-11-04 - First stable release

NoteHop has been public since 2026-10-27; this is the point where the features below became the base that later releases build on.

### Added

- **Notes.** Notes are locked in the browser by default, and the key stays in the link after the hash sign, so the server only stores text it cannot read. A basic note, readable by the server and with a link name you choose, is the other choice, and the owner can convert a note either way.
- **Editor.** Markdown with a slash menu, checklists, polls, forms, tables and diagrams, three note types (Plain, Document, Code), website notes that run in a sealed frame without internet, encrypted attachments that last one day, live editing with chat, and nearby devices.
- **Sharing and privacy.** Links with their own rights, owner links, copies frozen in time, delete after reading, expiry dates, decoy passwords, network allow lists, time capsules and sign-off.
- **Accounts.** One account for every HOP app through HOP ID, with recovery codes and a Security page to change the password. Notes also work without an account.
- **Help with writing.** Optional summarize, rewrite, translate and spelling help, using the browser's built-in AI or your own key.
- **Nine languages.** The interface, the READMEs and the documentation site are in English, Chinese, German, Indonesian, Japanese, Portuguese, Spanish, French and Russian.
- **Self-hosting.** One command installs, updates and removes NoteHop. It is a single program with a small database and daily backups.
- **Developer tools.** A command line, raw text addresses, a mock API, webhooks in and out, SSH, a Telegram bot and an installable app.

### Fixed

- **Reports.** A report no longer keeps a note's key, and it keeps only a scrambled form of the reporter's address.
- **Sign-in.** A sign-in for a suspended account is refused, and after signing in only addresses on this site are followed, which closes a way to be sent to another site.
- **Errors.** A page that fails now says so with an error instead of showing half a page, and a mistyped command no longer starts the server.
- **Deleting.** Deleting a note, an account or an expired note also deletes its attachments, and link names of deleted accounts stay reserved.
- **Live editing.** People are disconnected when a note is deleted or expires, and a person joining just as the last one leaves can no longer end up alone.
- **Settings and accessibility.** The "More settings" panel is a proper dialog, contrast is readable, controls are big enough on phones, and icon buttons have names.

### Security

- **Backups.** Backup files can be read only by their owner.
