package check

// before has two operations, one without an operationId. after removes the
// other and adds a path whose operation has no operationId either.
const before = `openapi: 3.0.3
info:
  title: Pets
  version: 1.0.0
  description: Pet store.
  contact:
    name: Team
servers:
  - url: https://api.example.com
tags:
  - name: pets
    description: Pets.
paths:
  /pets:
    get:
      summary: List pets
      tags: [pets]
      responses:
        '200':
          description: OK
    delete:
      operationId: deletePets
      summary: Delete pets
      description: Deletes pets.
      tags: [pets]
      responses:
        '204':
          description: Deleted
`

const after = `openapi: 3.0.3
info:
  title: Pets
  version: 1.0.0
  description: Pet store.
  contact:
    name: Team
servers:
  - url: https://api.example.com
tags:
  - name: pets
    description: Pets.
paths:
  /pets:
    get:
      summary: List pets
      tags: [pets]
      responses:
        '200':
          description: OK
  /owners:
    get:
      summary: List owners
      tags: [pets]
      responses:
        '200':
          description: OK
`
