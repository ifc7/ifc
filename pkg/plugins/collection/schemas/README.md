# Collection schemas

| File | Source | License |
| --- | --- | --- |
| `opencollection-1.0.0.schema.json` | `@opencollection/schema` 0.15.0 on npm (`src/opencollection.schema.json`, `$id` https://schema.opencollection.com/opencollection/v1.0.0.json), unchanged | MIT, Bruno Software Inc. |
| `postman-2.1.schema.json` | Written for ifc7 from the Postman Collection Format v2.1.0. It checks the structure the format requires and is more lenient than the official schema about optional fields. | Same as this repository |
| `json-schema-test-suite.schema.json` | Written for ifc7 from the JSON-Schema-Test-Suite file layout | Same as this repository |

To update OpenCollection: copy the new file from the npm package, bump
`openCollectionSchemaVersion` in `opencollection.go`, and run the tests.
