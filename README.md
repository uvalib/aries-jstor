# Aries JSTOR

This is an implementation of the Aries API: https://confluence.lib.virginia.edu/display/DCMD/Aries for JSTOR

### System Requirements
* GO version 1.12 or greater

### Current API

* GET /version : return service version info
* GET /healthcheck : test health of system components; results returned as JSON
* GET /api/aries/[ID] : Get information about objects contained in JSTOR that match the ID
