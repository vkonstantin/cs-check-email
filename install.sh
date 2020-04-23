#!/bin/sh

# input parameters
SERVICE=send-email

PROJECT_ID=`gcloud config list --format 'value(core.project)'`
ENV=`echo $PROJECT_ID | cut -d- -f1`
PROJECT_NUMBER=`gcloud projects list  | grep prod-205719 | awk '{print $3}'`
TOPIC=${SERVICE}_${ENV}

gcloud config set run/region us-east1
gcloud config set run/platform managed

# per project init
#gcloud projects add-iam-policy-binding $PROJECT_ID --member=serviceAccount:service-${PROJECT_NUMBER}@gcp-sa-pubsub.iam.gserviceaccount.com --role=roles/iam.serviceAccountTokenCreator
#gcloud iam service-accounts create cloud-run-pubsub-invoker --display-name "Cloud Run Pub/Sub Invoker"

# install
gcloud builds submit --tag gcr.io/$PROJECT_ID/$SERVICE
gcloud run deploy $SERVICE --image gcr.io/$PROJECT_ID/$SERVICE --platform managed

# per service init
gcloud pubsub topics create $TOPIC
gcloud run services add-iam-policy-binding $SERVICE --member=serviceAccount:cloud-run-pubsub-invoker@${PROJECT_ID}.iam.gserviceaccount.com --role=roles/run.invoker
SERVICE_URL=`gcloud run services list | grep $SERVICE | awk '{print $4}'`
echo $SERVICE_URL
gcloud pubsub subscriptions delete $TOPIC
gcloud pubsub subscriptions create $TOPIC --topic $TOPIC --push-endpoint=$SERVICE_URL --push-auth-service-account=cloud-run-pubsub-invoker@${PROJECT_ID}.iam.gserviceaccount.com --ack-deadline=60
