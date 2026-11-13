-- Bahasa Indonesia is a fourth display language (DEC-108): the two locale CHECKs accept 'id'.
ALTER TABLE identity.preferences DROP CONSTRAINT preferences_locale_check;
ALTER TABLE identity.preferences ADD CONSTRAINT preferences_locale_check CHECK (locale IN ('en','ms','ja','id'));
ALTER TABLE notify.templates DROP CONSTRAINT templates_locale_check;
ALTER TABLE notify.templates ADD CONSTRAINT templates_locale_check CHECK (locale IN ('en','ms','ja','id'));
