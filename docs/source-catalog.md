# Каталог публичных startup-источников

Проверено 10 июля 2026 года. Этот документ и [`source_catalog.json`](../backend/internal/ingestion/source_catalog.json) фиксируют разрешённую поверхность чтения, условия повторного использования, runtime mapping и fail-closed правила сетевых adapters.

## Утверждённые источники

| ID | Publisher evidence | Endpoint | Доступ | Cadence | Ожидаемая freshness |
|---|---|---|---|---:|---:|
| `innovate-uk` | [Страница Innovate UK](https://www.gov.uk/government/organisations/innovate-uk) объявляет `rel=alternate` Atom | `https://www.gov.uk/government/organisations/innovate-uk.atom` | public Atom, no auth | 60 min | ≤ 21 days |
| `uk-research-and-innovation` | [Страница UKRI](https://www.gov.uk/government/organisations/uk-research-and-innovation) объявляет `rel=alternate` Atom | `https://www.gov.uk/government/organisations/uk-research-and-innovation.atom` | public Atom, no auth | 60 min | ≤ 7 days |
| `british-business-bank` | [Страница British Business Bank](https://www.gov.uk/government/organisations/british-business-bank) объявляет `rel=alternate` Atom | `https://www.gov.uk/government/organisations/british-business-bank.atom` | public Atom, no auth | 60 min | ≤ 30 days |
| `hacker-news-show` | [Официальная документация Hacker News API](https://github.com/HackerNews/API) описывает `showstories` и item endpoints | `https://hacker-news.firebaseio.com/v0/showstories.json` | public JSON API, no auth | 60 min | ≤ 7 days |
| `techcrunch-startups` | [TechCrunch RSS Terms](https://techcrunch.com/rss-terms-of-use/) разрешают отображение feed content с атрибуцией и ссылкой на статью | `https://techcrunch.com/category/startups/feed/` | public RSS, no auth | 60 min | ≤ 7 days |
| `eu-startups` | [EU-Startups](https://www.eu-startups.com/about/) публично показывает RSS channel и publisher ownership, но не отдельную content-reuse лицензию | `https://www.eu-startups.com/feed/` | public RSS, factual metadata only | 60 min | ≤ 7 days |

На дату проверки все три publisher pages явно ссылались на соответствующий feed, а endpoints отвечали HTTP 200 с `application/atom+xml; charset=utf-8` без credentials. Adapter обязан разбирать media type отдельно от параметров MIME: изменение `charset` само по себе не является breaking change.

Контент GOV.UK разрешён к копированию, публикации, адаптации и коммерческому использованию по [Open Government Licence v3.0](https://www.nationalarchives.gov.uk/doc/open-government-licence/version/3/) при сохранении атрибуции. Логотипы, персональные данные, сторонние произведения и использование с намёком на официальное одобрение исключены. Поэтому каталог разрешает только title-derived metadata и короткий Atom summary, но не изображения и не полный article body.

GOV.UK request использует узнаваемый User-Agent, timeout 10 секунд, предел 1 MiB, максимум 100 entries и не чаще одного раза в час. Разрешены максимум три redirect; final URL обязан остаться HTTPS на `www.gov.uk`. Article HTML, search pages, email subscriptions, login и иные endpoints не используются как fallback.

Show HN adapter использует официальный JSON API: один `showstories` request и не более 40 item requests за цикл. Каждый body ограничен 64 KiB, request timeout — 5 секунд, общий deadline — 15 секунд, redirect запрещён. Разрешены только HTTPS host `hacker-news.firebaseio.com` и точные `/v0/showstories.json`/`/v0/item/<id>.json`; product URL никогда не fetch-ится. Submitter, comments, score, story text и raw JSON не декодируются и не сохраняются.

TechCrunch и EU-Startups используют общий bounded RSS adapter: timeout 10 секунд, предел 1 MiB, максимум 50 items, не более трёх redirect и одного request в час. Каждый redirect и final URL обязан оставаться HTTPS на точном publisher host. Article links сохраняются только для атрибуции и никогда не fetch-ятся. Mapper игнорирует description, author, `content:encoded`, images, tracking markup и raw XML; остаются только независимо извлечённые headline facts и publisher link.

TechCrunch RSS Terms требуют не изменять отображаемый feed content, поэтому digest не копирует и не преобразует `item.description`. EU-Startups не публикует отдельную RSS reuse-лицензию: `reuse_verified=false`, а разрешённый режим ограничен factual metadata и ссылкой без воспроизведения feed text. Для обоих sources publisher description в `SourceRecord` остаётся пустым.

## Runtime enablement и отключение

Безопасный default — dry-run: `config.Default()` регистрирует только локальный `sample-public` и не выполняет live HTTP fetch. Сеть включается только явным opt-in:

```bash
DAILY_STARTUPS_DRY_RUN=false make run-backend
```

В live mode embedded catalog создаёт registry и шесть активных source configuration. `sample-public` в live mode запрещён. `DAILY_STARTUPS_SOURCES_JSON` не задаёт URL, display name, cadence, limits или tags: это строгий activation overlay. Например, следующая конфигурация отключает Innovate UK, оставляя неуказанные источники активными:

```bash
DAILY_STARTUPS_DRY_RUN=false \
DAILY_STARTUPS_SOURCES_JSON='[{"id":"innovate-uk","active":false,"access_method":"atom"}]' \
make run-backend
```

Duplicate/unknown IDs, credentials и несовпадение approved access method завершают startup до открытия SQLite и HTTP listener. Catalog metadata всегда перезаписывает одноимённые поля overlay, поэтому конфигурацией нельзя подменить publisher, cadence или rate limit.

Scheduler — единственный runtime consumer, который запускает ingestion fetch. Перед HTTP request он атомарно сохраняет отдельный `last_attempt_at`; этот timestamp переживает restart и переход health в `skipped`, поэтому crash-loop или быстрое disable/re-enable не обходят 60-минутный cadence. Если reservation нельзя записать, network request не выполняется. `/v1/digests/preview` выбирает сохранённые signals за локальные календарные сутки запрошенного timezone и никогда не инициирует сетевой запрос. Отключённый source записывает health status `skipped`, заменяя прежний failure; `skipped` не переводит общий `/health` в `degraded`.

## Quality gate и дедупликация

До persistence каждый production record проходит immutable policy своего adapter. Максимальный возраст берётся только из catalog: Innovate UK — 504 часа, UKRI — 168 часов, British Business Bank — 720 часов, Show HN — 168 часов, TechCrunch Startups — 168 часов, EU-Startups — 168 часов; `serve_stale_as_new` обязан оставаться `false`. Timestamp более чем на 15 минут в будущем также отклоняется. Отсутствующие source ID, company name, exact HTTPS source URL или publication time не дополняются fetch time.

Каждый skip учитывается без raw content: adapter-level rejection, quality reason (`missing_*`, `invalid_*`, `stale`, `future`) и storage failure имеют отдельные counters. Для активного source выполняются инварианты `fetched = normalized + skipped` и `skipped = adapter_skipped + quality_rejected`; ошибка SQLite не маскируется как плохой upstream record.

URL identity удаляет только однозначные tracking keys (`utm_*`, `gclid`, `fbclid`, `msclkid`, `mc_cid`, `mc_eid`, `_hs*`), сохраняя функциональные query parameters, включая `ref`. Attribution продолжает использовать точный publisher URL. Digest объединяет exact company names в пределах запрошенного local digest day/region; fuzzy matching не используется. Legal suffix удаляется только при одинаковом source-event URL или точном непустом funding amount+currency. Два разных canonical URLs всегда остаются разными, а unanchored alias не может связать их транзитивно.

## Общий контракт `SourceRecord`

| Поле | Источник и правило | Если нет уверенности |
|---|---|---|
| `startup_name` | GOV.UK: одна компания/spinout до approved event verb. Show HN: строгий prefix `Show HN:`. Publisher RSS: одна явно названная компания рядом с funding/launch/market-entry verb; bounded `*-based` и `startup/scaleup` prefix удаляется, generic noun phrase отклоняется. | Item пропускается. |
| `canonical_url` | Только абсолютный HTTPS product URL из HN item; URL не fetch-ится. Publisher article не считается homepage. | Пустая строка. |
| `source_url` | GOV.UK alternate link, HN discussion URL либо точный TechCrunch/EU-Startups `item.link`. | Item пропускается. |
| `signal_type` | GOV.UK — явный event verb; Show HN — `launch`; publisher RSS — `funding` или `launch`, включая явный market entry. | Item пропускается либо `news` только для принятого GOV.UK event. |
| `published_at` | Atom `entry.updated`, RSS `item.pubDate` либо HN Unix `time`, затем UTC. Fetch time не подставляется. | Item пропускается. |
| `description` | GOV.UK: очищенный feed summary до 280 символов. Show HN: только bounded tagline из title. TechCrunch/EU-Startups: не копируется из-за reuse policy. | Пустая строка. |
| `region` | Scope GOV.UK, адрес publisher или university location не доказывают регион компании. | Пустая строка. |
| `categories` | В organization feeds нет стабильной product taxonomy. | Пустой список. |
| `funding` | Amount/currency/round — только если они явно присутствуют в headline; investors не структурированы. | Пустая структура. |
| `raw_payload` | Adapter оставляет поле пустым. `NormalizeSignal` создаёт bounded JSON только из typed `categories` и `funding`; raw Atom/JSON body не хранится. | Пустая строка. |

`entry.id` используется для проверки upstream identity и диагностики, но не как `canonical_url`. Неопределённые значения никогда не дополняются LLM, geocoding, article scraping или догадкой из publisher scope.

## Admission rules по источникам

### Innovate UK

- Принимается одна названная компания с concrete funding, launch, award или commercial milestone.
- Programme announcements, aggregate project lists, policy, advice, recruitment и reports пропускаются.
- Публикация Innovate UK сама по себе не доказывает, что субъект — startup: headline должен пройти company/event gate.

### UK Research and Innovation

- Принимается одна названная компания или spinout с launch, funding, award, acquisition или commercial milestone.
- University/research aggregates, grants без одной компании, consortium announcements, events, reports и jobs пропускаются.
- University location не переносится в `region` компании.

### British Business Bank

- Принимается одна operating company с concrete raise, investment, guarantee, launch, acquisition или portfolio milestone.
- Funds, schemes, lenders, market reports, statistics, appointments и aggregate portfolios не считаются startup records.
- Слова `business`, `bank`, `fund` или `portfolio` без однозначного company subject не проходят admission.

### Hacker News Show HN

- Принимаются только живые `story` items со строгим prefix `Show HN:`.
- Короткое product name принимается отдельно либо перед явным dash/pipe separator; first-person и sentence-like subjects пропускаются.
- `by`, `text`, `kids`, comments и score игнорируются. Unsafe/non-HTTPS product URL очищается, но корректный discussion URL сохраняется.

### TechCrunch Startups RSS

- Принимается одна явно названная компания с `raises`, `secures`, `closes`, `launches`, `debuts`, `enters ... market` или `expands into/across` событием; generic `new AI platform` не считается company name.
- Funding требует amount, явный `Series X`/`... round` или funding/investment context; слова `growth`/`seed` вне round phrase не заполняют round.
- `launches`/`debuts` требуют reviewed product object (`platform`, `software`, `app`, `model`, `tool` и т. п.); campaign, podcast и promotion отклоняются.
- Round-ups, lists, advice, opinion, events, funds, M&A, people stories и multi-company subjects пропускаются.

### EU-Startups RSS

- Применяется тот же fail-closed event grammar; location-based prefix разрешено удалить только до одной компании.
- Заголовок не используется для вывода `region`: publisher scope и `Barcelona-based` не доказывают operating region.
- Summit/event promotion, fund announcements, acquisitions, lists и неоднозначные subjects не создают records.
- Description и остальной publisher text не копируются: только factual headline metadata и exact article link.

Полные source-specific rules и ожидаемый `SourceRecord` находятся в machine-readable catalog. Шесть synthetic fixtures повторяют актуальные Atom/RSS/JSON shapes, но не копируют production content:

- [`innovate-uk.xml`](../backend/internal/ingestion/testdata/source_catalog/innovate-uk.xml)
- [`uk-research-and-innovation.xml`](../backend/internal/ingestion/testdata/source_catalog/uk-research-and-innovation.xml)
- [`british-business-bank.xml`](../backend/internal/ingestion/testdata/source_catalog/british-business-bank.xml)
- [`hacker-news-show.json`](../backend/internal/ingestion/testdata/source_catalog/hacker-news-show.json)
- [`techcrunch-startups.xml`](../backend/internal/ingestion/testdata/source_catalog/techcrunch-startups.xml)
- [`eu-startups.xml`](../backend/internal/ingestion/testdata/source_catalog/eu-startups.xml)

Offline contract test пропускает каждую fixture через настоящий runtime adapter и сравнивает все поля `SourceRecord` с `fixture_expected`, включая empty collections и `RawPayload`.

## Attribution, хранение и удаление

Каждый отображаемый signal называет publishing organisation и даёт прямую ссылку `source_url`. GOV.UK дополнительно показывает `OGL v3.0 · нормализованное резюме`; Show HN — `HN API · публичная публикация`; publisher RSS — `TechCrunch RSS · headline metadata` или `EU-Startups RSS · headline metadata`. Структурные `source_id`/`source_url` сохраняются в immutable digest snapshot, поэтому retry и delivery после перезапуска не теряют attribution. Feed content не выдаётся за собственный оригинальный материал; логотипы, images и article body не зеркалируются.

Source немедленно отключается для новых fetch и public display, если publisher убрал Atom discovery, изменил reuse terms, потребовал auth, запретил такой доступ или attribution больше нельзя сохранить. Исторические `source_id`/`source_url` могут оставаться только во внутреннем audit; прежние публичные digests не считаются разрешённым fallback cache.

## Degradation и breaking changes

- Network/timeout/size/XML/JSON failure деградирует логический source; остальные продолжают ingestion.
- Непустой цикл с `normalized=0` получает status `zero_yield` и переводит общий health в `degraded`. Пустой feed, cadence skip и partial yield остаются healthy.
- Все три GOV.UK feeds работают на одной platform, поэтому её outage может затронуть их одновременно. TechCrunch и EU-Startups остаются отдельными logical sources; scheduler считает health каждого независимо.
- Нет fallback на HTML scraping, cached body как новый signal или другой неутверждённый endpoint.
- Retry выполняется на следующем cadence с bounded backoff; старые signals не переиздаются с новой датой.
- Неожиданный Atom root/namespace, пропажа `entry.title`, alternate link или `entry.updated`, смена final host/media type либо массовое падение admission — breaking change.
- Breaking format принимается только после совместного обновления fixture, catalog mapping, reuse review и tests. До этого adapter fail-closed.

Проверка каталога входит в обычный `go test ./...` и не обращается к сети. Live access/reuse probe остаётся отдельной operator/review процедурой: не более одного request на source в 60 минут, без автоматического retry и без запуска в CI. Probe фиксирует HTTP/media type и adapter accounting; отсутствие принятого startup item не является transport failure и не смягчает admission rules.

Последний ручной production-adapter probe выполнен 10 июля 2026 года в 05:20 UTC, по одному request на endpoint: Innovate UK — `20 fetched / 20 skipped`, UKRI — `20 / 20`, British Business Bank — `19 / 19`. Все три transport/parser results имели status `ok`; текущие entries не прошли строгий single-company event gate, поэтому `normalized=0` и синтетические startup signals не создавались.

Bounded RSS transport probe выполнен 10 июля 2026 года по одному GET на новый endpoint без retry: TechCrunch Startups — HTTP 200, `application/rss+xml`, 18 items / 18 с обязательными полями, 18,526 bytes; EU-Startups — HTTP 200, `application/rss+xml`, 10 / 10, 81,962 bytes. Оба ответа уложились в 10 секунд, 1 MiB и 50 items, не потребовали redirect и завершились на точных HTTPS publisher hosts.

Отдельный bounded Show HN probe выполнен 10 июля 2026 года в 14:31 UTC на временной SQLite с отключёнными GOV.UK sources: `40 fetched / 26 normalized / 26 stored / 14 skipped`, status `ok`. Preview вернул 10 items с `HN API · публичная публикация`, без OGL, submitter/comment/story-text полей и raw API JSON. Временный backend и база после проверки удалены.

Финальный scheduled-delivery probe выполнен 10 июля 2026 года в 14:42 UTC на отдельной временной SQLite: live ingestion снова дал `40 / 26 / 26 / 14`, scheduler создал один digest из 9 реальных items, delivery queue перешла в `sent`, а Telegram attempt — в `success`. Полученное сообщение содержало 9 прямых Show HN discussion links и корректную `HN API · публичная публикация` attribution. Основные subscriber preferences не изменялись; временное состояние удалено.
