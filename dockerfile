FROM alpine:latest
WORKDIR /app
COPY nubewatch .
RUN mkdir /app/data
RUN touch /app/data/.keep
ENTRYPOINT ["/app/nubewatch"]
