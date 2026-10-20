---
title: Deploy to production
parent: Runbooks
position: 1
author: s-tomasz
created: -27d 09:30
labels: [runbook, deploy]
edited: -8d 14:20
editor: s-mateo
message: The canary waits for traffic at night, and the rollback is one command
versions:
  - at: -27d 09:30
    by: s-tomasz
    message: First version, moved over from the old notes
    body: |
      ```bash
      alp deploy summit --env prod
      ```

      Watch the dashboard for 15 minutes. If errors go up, deploy the previous version again.
  - at: -18d 10:15
    by: s-lena
    message: Smoke tests before production
    body: |
      ![Deploy pipeline](lib:deploy-pipeline)

      ## Steps

      1. Merge to main. Tests and the image build run by themselves.
      2. Staging deploys by itself. Run the smoke tests:

         ```bash
         alp smoke --env staging
         ```

      3. Deploy to production:

         ```bash
         alp deploy summit --env prod
         ```

      4. Watch the dashboard for 15 minutes. If errors go up, deploy the previous version again.
---
![Deploy pipeline](lib:deploy-pipeline)

## Before you start

- Not on Friday after 15:00, and not during the first hour of a sale
- Someone else knows you are deploying

## Steps

1. Merge to main. Tests and the image build run by themselves.
2. Staging deploys by itself. Run the smoke tests:

   ```bash
   alp smoke --env staging
   ```

3. Deploy to production. The canary takes 10 % of traffic for 15 minutes, or until it has seen 500 requests:

   ```bash
   alp deploy summit --env prod --canary 10 --min-requests 500
   ```

4. The pipeline promotes the canary when its error rate stays below 1 %. Watch the [[page:On-call]] dashboard while it does.

## Rolling back

```bash
alp deploy rollback summit --env prod
```

The rollback deploys the previous image to everyone at once. It takes about two minutes. Migrations are never rolled back by a deploy: every migration must work with the previous version of summit.

Known problem: at night the canary may wait a long time for traffic, see [[OPS-42]].
