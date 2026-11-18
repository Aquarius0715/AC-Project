"""Test of the identity provider's themes (IR646, IR789, IR795): every login and e-mail theme offers each language the
realm turns on, and each login theme has every product text in each of them with the same placeholders as English.

usage: python3 -m unittest docker/keycloak/test_themes.py
"""

import glob
import json
import os
import re
import unittest

HERE = os.path.dirname(os.path.abspath(__file__))
THEMES = os.path.join(HERE, "themes")
PLACEHOLDER = re.compile(r"\{\d+\}")


def properties(path: str) -> dict:
    """The key=value lines of a .properties file (the themes use no line continuations)."""
    out = {}
    with open(path, encoding="utf-8") as f:
        for line in f:
            line = line.strip()
            if not line or line.startswith("#"):
                continue
            key, sep, value = line.partition("=")
            if not sep:
                raise ValueError(f"{path}: not key=value: {line}")
            out[key] = value
    return out


def realm_locales() -> list:
    with open(os.path.join(HERE, "realm-ac.json"), encoding="utf-8") as f:
        return json.load(f)["supportedLocales"]


def theme_dirs() -> list:
    """Each theme type folder (themes/<name>/<login|email>) with its theme.properties."""
    found = sorted(os.path.dirname(p) for p in glob.glob(os.path.join(THEMES, "*", "*", "theme.properties")))
    if not found:
        raise AssertionError(f"no theme.properties under {THEMES}")
    return found


class ThemeTest(unittest.TestCase):
    def test_every_theme_offers_the_realm_languages(self):
        want = realm_locales()
        for folder in theme_dirs():
            with self.subTest(theme=os.path.relpath(folder, THEMES)):
                got = properties(os.path.join(folder, "theme.properties")).get("locales", "").split(",")
                self.assertEqual(want, got)

    def test_login_texts_exist_in_each_language_with_the_same_placeholders(self):
        locales = realm_locales()
        for folder in theme_dirs():
            messages = os.path.join(folder, "messages")
            if not os.path.isdir(messages) or os.path.basename(folder) != "login":
                continue
            english = properties(os.path.join(messages, "messages_en.properties"))
            for loc in locales:
                with self.subTest(theme=os.path.relpath(folder, THEMES), locale=loc):
                    texts = properties(os.path.join(messages, f"messages_{loc}.properties"))
                    self.assertEqual([], sorted(set(english) - set(texts)), "texts missing")
                    for key, value in english.items():
                        self.assertEqual(
                            sorted(PLACEHOLDER.findall(value)), sorted(PLACEHOLDER.findall(texts[key])), key
                        )

    def test_the_chooser_names_each_language_in_every_language(self):
        locales = realm_locales()
        messages = os.path.join(THEMES, "ac", "login", "messages")
        for loc in locales:
            with self.subTest(locale=loc):
                texts = properties(os.path.join(messages, f"messages_{loc}.properties"))
                self.assertEqual([], [f"locale_{x}" for x in locales if not texts.get(f"locale_{x}")])


if __name__ == "__main__":
    unittest.main()
