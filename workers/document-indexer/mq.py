from __future__ import annotations

import json
from typing import Callable

import pika

from config import WorkerConfig


class DocumentMQ:
    def __init__(self, config: WorkerConfig):
        self.config = config
        self.connection = pika.BlockingConnection(pika.URLParameters(config.rabbitmq_url))
        self.channel = self.connection.channel()
        self.declare_topology()

    def declare_topology(self) -> None:
        self.channel.exchange_declare(
            exchange=self.config.exchange,
            exchange_type="direct",
            durable=True,
        )
        self.channel.queue_declare(queue=self.config.queue, durable=True)
        self.channel.queue_bind(
            queue=self.config.queue,
            exchange=self.config.exchange,
            routing_key=self.config.routing_key,
        )
        self.channel.basic_qos(prefetch_count=1)

    def publish_event(self, event: dict) -> None:
        body = json.dumps(event, ensure_ascii=False).encode("utf-8")
        self.channel.basic_publish(
            exchange=self.config.exchange,
            routing_key=self.config.routing_key,
            body=body,
            properties=pika.BasicProperties(
                content_type="application/json",
                delivery_mode=2,
            ),
        )

    def close(self) -> None:
        if self.connection and self.connection.is_open:
            self.connection.close()

    def consume(self, handler: Callable[[dict, pika.channel.Channel, pika.spec.Basic.Deliver], None]) -> None:
        def on_message(channel, method, properties, body):
            try:
                event = json.loads(body.decode("utf-8"))
            except Exception:
                channel.basic_ack(delivery_tag=method.delivery_tag)
                return
            handler(event, channel, method)

        self.channel.basic_consume(
            queue=self.config.queue,
            on_message_callback=on_message,
            auto_ack=False,
        )
        self.channel.start_consuming()
