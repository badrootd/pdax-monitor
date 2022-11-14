# PDAX volume monitor

[PDAX](https://pdax.ph/) is Cryptocurrency Trading Platform in the Philippines.

We need to monitor trading volume for specific [pairs](auxiliary/currencyCodes.json).

*Important*

PDAX is not providing official API and `volume-pdax-monitor` service is using unofficial websocket API from (PDAX trading terminal)[https://trade.pdax.ph].

It means that if protocol will be changed we will need to adjust service.

### Schema

![Schema](schema.png)

### Requirements

* User in PDAX. Not need to pass KYC, after registration user has access to (trading terminal)[https://trade.pdax.ph].
* Account in (2Captcha)[https://2captcha.com/]. This service is using for solving captcha during login to PDAX.

For create accounts was used service for generate temporary anonynous emails - (TempMmail)[https://temp-mail.org/].

### Deploy

Currently service in running on EC2 instance in prod account because websocket connection is termitating with 1006 error when service is running in kubernetes.

Ideally we should to migrate service to kubernetes.

