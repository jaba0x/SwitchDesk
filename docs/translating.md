# Translating SwitchDesk

SwitchDesk can show its interface in other languages. The translations are made by people who use it, and every language is welcome. You don't need to know how to program: a translation is one text file.

## Pick a language

**Settings → Language** lists the languages that come with SwitchDesk. **Automatic** uses your computer's language when SwitchDesk has it, and English otherwise.

Switch output always stays as the switch sends it: configs, command output, log lines, port names and addresses. So do the names you give your switches.

## How a translation file looks

Every language is a JSON file named after its language code, such as `de.json` for German or `pt-BR.json` for Brazilian Portuguese:

```json
{
  "language": "Deutsch",
  "code": "de",
  "strings": {
    "Add switch": "Switch hinzufügen",
    "{0} of {1} online": "{0} von {1} online",
    "Settings": "Einstellungen"
  }
}
```

- The **key** on the left is the English text, exactly as SwitchDesk shows it. Don't change it.
- The **value** on the right is your translation. Leave it empty (`""`) to keep the English text for now. A half-finished file already helps.
- **`{0}`, `{1}`…** stand for things that change, such as numbers, switch names and versions. Keep every one of them in your translation, in whatever order your language needs: `"{0} of {1} online"` can become `"{1} 台中 {0} 台在线"`.
- `language` is the name shown in the list, written in the language itself (`Deutsch`, `Français`, `ქართული`).

The full list of texts is [`frontend/dist/i18n/en.json`](https://github.com/jaba0x/SwitchDesk/blob/main/frontend/dist/i18n/en.json), with every value empty. Start from a copy of it.

## Try it in SwitchDesk

You can try a translation without building anything:

1. In **Settings → Language**, click **Translation files**. This opens the `translations` folder inside your data folder.
2. Put your file there, for example `fr.json`.
3. Choose it in **Settings → Language**. It's marked "your file".

Edit the file, then pick the language again to see your changes. A file in this folder also overrides the built-in translation with the same code, which is handy for fixing a word.

## Share it

So everyone gets your language:

- **Through GitHub:** fork the repository, add or edit `frontend/dist/i18n/<code>.json`, add your language to `frontend/dist/i18n/languages.json`, and open a pull request.
- **Without GitHub tools:** attach the file to the [translation issue](https://github.com/jaba0x/SwitchDesk/issues?q=is%3Aissue+translate) or email it to jaba@clihero.com.

You're credited in the changelog and the README.

## Reviewing a draft

Some languages are marked **draft**. A draft was translated without a native speaker yet, so it may have stiff wording or wrong network terms. Reading through it in SwitchDesk and fixing what sounds wrong is one of the most useful things you can do. Many network terms (VLAN, trunk, untagged, uplink) are usually left in English; follow what your colleagues actually say.

## For developers

The page is written in English, and `frontend/dist/i18n.js` swaps in the chosen language as it's drawn, so new code needs nothing special. After changing interface text, update the list for translators:

```sh
cd tools/i18n
npm install
npm run extract
```

Text that Go puts together at run time, which the extractor can't see whole, goes into `tools/i18n/extra.txt` as a pattern. To find text that still has no translation, open SwitchDesk in a language, use it for a bit, then run `I18N.missing()` in the developer console of `frontend/dev/preview.html`.
