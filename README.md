# Chirpy

Chirpy is a social network similar to Twitter. It utilizes API endpoints for user, chirps, authentication & authorization tokens, hashed passwords, and webhooks. 

## Endpoints
Some endpoints are authenticated endpoints and require an Authorization header in the request. Below is a list of endpoints and their functionalities:
#### User Endpoints
`POST /api/users` - creates a new user; requires an Email & Password in request body; hashes password before storing in database\
`PUT /api/users` - (authenticated) updates user information (requires Email & Password in request body) for current user\
`POST /api/login` - logs in user provided required Email & Password in request body; creates a 1hr JWT and 60 day refresh token for the user if login is successful
#### Chirp Endpoints
`GET /api/chirps?author_id?sort` - lists all chirps in database; can be filtered/sorted with optional query parameters
- author_id (optional) -> only shows chirps for provided author_id
- sort (optional) -> sorts by created_at timestamp ascending (default) or descending\
<!-- end of the list -->
`GET /api/chirps/{chirpID}` - lists chirp information by provided chirp's ID\
`POST /api/chirps` - (authenticated) - creates a new chirp with provided information in request body (requires body) for currently logged-in user\
`DELETE /api/chirps/{chirpID}` - (authenticated) deletes chirp with provided ID if the author is the currently logged-in user

#### Webhooks
`POST /api/polka/webhooks` - (authenticated) provided that the ApiKey in the Authorization header matches the .env ApiKey, then the user is upgraded to Chirpy Red provided the event in the request body is user.upgraded

#### Miscellaneous Endpoints
`POST /admin/reset` - resets server's request counter and deletes all users from database; useful for testing\
`GET /admin/metrics` - prints server request counter in an HTML body\
`GET /api/healthz` - tester endpoint; prints "OK" in body and returns a 200 status code\
`POST /api/refresh` - (authenticated) creates a new access token for current user if refresh token is valid (not expired nor revoked)\
`POST /api/revoke` - (authenticated) revokes refresh token access for provided token in Authorization header

## Schema
Below is the schema structures for the 3 database tables utilized:
#### Users
id | created_at | updated_at | email | hashed_password | is_chirpy_red
:-----: | :-----: | :-----: | :-----: | :-----: | :-----: 
UUID | TIMESTAMP | TIMESTAMP | TEXT | TEXT | BOOLEAN

#### Chirps
id | created_at | updated_at | body | user_id
:-----: | :-----: | :-----: | :-----: | :-----:
UUID | TIMESTAMP | TIMESTAMP | TEXT | UUID 

#### Refresh Tokens
token | created_at | updated_at | user_id | expires_at | revoked_at
:-----: | :-----: | :-----: | :-----: | :-----: | :-----:
TEXT | TIMESTAMP | TIMESTAMP | UUID | TIMESTAMP | TIMESTAMP