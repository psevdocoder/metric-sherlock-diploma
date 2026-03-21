# metric-sherlock-diploma

## HTTP API

API описан в protobuf-контракте: `proto/metricsherlock/targetgroups/v1/target_groups.proto`.
Kafka событие факта проверки нарушений описано в protobuf-контракте:
`proto/metricsherlock/metricviolations/v1/metric_violation_fact.proto`.

### Endpoints

- `GET /api/v1/target-groups` - список всех target-групп.
- `GET /api/v1/target-groups?team_name=<team>` - список target-групп конкретной команды.
- `GET /api/v1/target-groups/{id}` - детальная информация по target-группе, включая статистику нарушений.

### Swagger

- JSON спецификация: `GET /swagger/target-groups.json`
- UI: `GET /swagger/`

### Генерация proto/Swagger

```bash
make proto
```
