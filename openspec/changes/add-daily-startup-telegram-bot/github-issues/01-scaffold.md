Part of #1

## Контекст

Репозиторий пока содержит только OpenSpec/Codex scaffolding. Нужно создать базу приложения, на которой дальше будут строиться storage, Telegram, ingestion и digest.

## Задача

Создать Kotlin/JVM Gradle приложение с entry point, базовыми Gradle tasks, зависимостями, sample config и smoke test.

## Acceptance criteria

- [ ] Создана Gradle/Kotlin структура проекта.
- [ ] Есть application entry point.
- [ ] Добавлены зависимости для coroutines, HTTP client, serialization, SQLite, logging и tests.
- [ ] Есть sample local config без секретов.
- [ ] `./gradlew test` проходит.
- [ ] `./gradlew build` проходит.

