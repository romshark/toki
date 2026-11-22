# Domains

Toki supports [TIK domains](https://github.com/romshark/tik/blob/main/SPECIFICATION.md#domains) through `.tokidomain.yml` files. A domain names a directory tree of your module and describes how to translate its texts, such as the expected tone, style, target audience or terminology conventions. `toki edit` groups the TIKs by domain.

> [!NOTE]
> Toki doesn't pass the description of a domain on to translators yet. It isn't in the `.arb` files, and `toki edit` doesn't show it.

`toki generate` creates a `.tokidomain.yml` file at your module root whenever there is none, named after the last element of the module path: `myapp` for `github.com/me/myapp`. `toki lint` doesn't create it.

A `.tokidomain.yml` file is a simple YAML file:

```yaml title=".tokidomain.yml"
# The TIK domain for this directory tree.
# The description provides context for translators working on this domain.
# Also see: https://github.com/romshark/tik/blob/main/SPECIFICATION.md#domains

# This defines the name of the domain.
name: myapp

# The description provides translators with additional context and instructions on
# how to translate within a directory tree, such as the expected tone, style,
# target audience, or terminology conventions.
description: ""
```

## Sub-domains

Domains can be nested. Place additional `.tokidomain.yml` files in subdirectories to create sub-domains:

```
myapp/
├─ .tokidomain.yml # myapp
└─ internal/
   ├─ storefront/
   │  ├─ .tokidomain.yml # myapp.storefront
   │  ├─ checkout/
   │  │  └─ .tokidomain.yml # myapp.storefront.checkout
   │  └─ catalog/ # (inherits myapp.storefront)
   └─ admin/
      └─ .tokidomain.yml # myapp.admin
```

Each file names only its own part, as `name: storefront` in `internal/storefront/`. The full name of a sub-domain joins the names of its parents and its own with `.`, as `myapp.storefront`.

Directories without a `.tokidomain.yml` file inherit the nearest ancestor's domain. A sub-domain is a distinct domain from its parent.

## Changing domains

The ID of a message in the catalogs depends on the full name of its domain.

> [!CAUTION]
> Adding a `.tokidomain.yml` file, renaming a domain or moving a TIK into another domain gives its messages new IDs. `toki generate` then adds the messages under their new IDs, blank in the catalogs of other languages, and removes the old ones with their translations.

## A TIK in several domains

The bundle looks up translations by TIK and can't tell domains apart. `toki generate` and `toki lint` report an error when a TIK is used in more than one domain, such as in `myapp` and `myapp.storefront`. Use a [context](https://github.com/romshark/tik/blob/main/SPECIFICATION.md#context) to tell them apart, such as `Order` and `[storefront] Order`.
