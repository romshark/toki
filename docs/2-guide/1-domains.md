# Domains

Toki supports [TIK domains](https://github.com/romshark/tik/blob/main/SPECIFICATION.md#domains) through `.tokidomain.yml` files. A domain provides translators and LLMs with additional context and instructions on how to translate within a directory tree, such as the expected tone, style, target audience or terminology conventions.

When you run `toki generate` for the first time, it creates a `.tokidomain.yml` file at your module root, named after the Go module.

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
   │  │   └─ .tokidomain.yml # myapp.storefront.checkout
   │  └── catalog/ # (inherits myapp.storefront)
   └─ admin/
      └─ .tokidomain.yml # myapp.admin
```

Directories without a `.tokidomain.yml` file inherit the nearest ancestor's domain. A sub-domain is a distinct domain from its parent.

## A TIK in several domains

The bundle looks up translations by TIK and can't tell domains apart. `toki generate` reports an error when a TIK is used in more than one domain, such as in `myapp` and `myapp.storefront`. Use a [context](https://github.com/romshark/tik/blob/main/SPECIFICATION.md#context) to tell them apart, such as `Order` and `[storefront] Order`.
