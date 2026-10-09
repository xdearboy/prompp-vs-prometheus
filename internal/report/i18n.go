package report

import "fmt"

func (b *Bundle) t(s string) string {
	return translate(b.Lang, s)
}

func translate(lang, s string) string {
	if lang != "ru" {
		return s
	}
	if v, ok := ru[s]; ok {
		return v
	}
	return s
}

func switcher(lang, name string) string {
	if lang == "ru" {
		return fmt.Sprintf("[English](%s.md) · **Русский**\n\n", name)
	}
	return fmt.Sprintf("**English** · [Русский](%s.ru.md)\n\n", name)
}

var ru = map[string]string{
	"%s free of %s":              "свободно %s из %s",
	"# Aggregate of %d runs\n\n": "# Сводка по %d прогонам\n\n",
	"Each cell is the median over runs with the minimum and maximum, and the spread as the share of the median.\n\n": "В каждой ячейке медиана по прогонам, рядом минимум, максимум и разброс в процентах от медианы.\n\n",
	"| engine | runs | median | min to max | spread |\n|---|---|---|---|---|\n":                                      "| движок | прогонов | медиана | от минимума до максимума | разброс |\n|---|---|---|---|---|\n",
	"to":                                "до",
	"RSS at the last step":              "RSS на последней ступени",
	"Working set at the last step":      "Рабочий набор на последней ступени",
	"Ingest cpu cores, average":         "Процессор при записи, ядер в среднем",
	"Data directory":                    "Каталог данных",
	"Query geomean p50, concurrency %d": "Время запросов, геометрическое среднее p50, параллельность %d",
	"# Prom++ versus Prometheus\n\n":    "# Prom++ против Prometheus\n\n",
	"Generated from the raw artifacts in this directory. Every number below comes from a file in the run directory, nothing is typed in by hand.\n\n": "Создано из исходных файлов этой папки. Каждое число ниже взято из файла прогона, вручную ничего не вписано.\n\n",
	"Run id: `%s`\n\n":              "Номер прогона: `%s`\n\n",
	"## Environment\n\n":            "## Окружение\n\n",
	"| item | value |\n|---|---|\n": "| параметр | значение |\n|---|---|\n",
	"node":                          "узел", "cpu": "процессор", "cores": "ядер", "memory": "память", "kernel": "ядро", "os": "ОС",
	"arch": "архитектура", "data filesystem": "файловая система данных", "harness commit": "коммит нагрузочного кода", "harness": "нагрузочный код",
	"\n## Engines\n\n": "\n## Движки\n\n",
	"Declared settings come from the manifests, observed ones from the engine's own runtimeinfo and flags endpoints during the run.\n\n":                                            "Заданные настройки взяты из манифестов, наблюдаемые из runtimeinfo и списка флагов самого движка во время прогона.\n\n",
	"| engine | image | cpu limit | memory limit | env | observed GOMEMLIMIT | observed GOMAXPROCS | WAL compression | args | notes |\n|---|---|---|---|---|---|---|---|---|---|\n": "| движок | образ | лимит процессора | лимит памяти | окружение | GOMEMLIMIT (факт) | GOMAXPROCS (факт) | сжатие WAL | аргументы | заметки |\n|---|---|---|---|---|---|---|---|---|---|\n",
	"\n### Notes\n\n":               "\n### Примечания\n\n",
	"## Ingest and head memory\n\n": "## Запись и память head\n\n",
	"![compared with Prometheus 3.15.0](charts/summary.svg)\n\n":                 "![в сравнении с Prometheus 3.15.0](charts/summary.svg)\n\n",
	"![resident memory by series](charts/rss-by-series.svg)\n\n":                 "![память процесса по числу серий](charts/rss-by-series.svg)\n\n",
	"![cpu cores by series](charts/cpu-by-series.svg)\n\n":                       "![процессор по числу серий](charts/cpu-by-series.svg)\n\n",
	"![resident memory over time](charts/rss-timeline.svg)\n\n":                  "![память процесса во времени](charts/rss-timeline.svg)\n\n",
	"![data directory by series](charts/disk-by-series.svg)\n\n":                 "![каталог данных по числу серий](charts/disk-by-series.svg)\n\n",
	"Interval %gs, batch %d series, %d workers, %d samples sent, %d failed.\n\n": "Интервал %g с, пачка %d серий, потоков %d, отправлено точек %d, из них потеряно %d.\n\n",
	"| active series | samples/s | head series | head chunks | RSS avg | RSS p95 | RSS max | working set max | cpu cores avg | cpu cores max | data dir | WAL | rss/series | disk/series |\n": "| активных серий | точек/с | серий в head | чанков | RSS ср. | RSS p95 | RSS макс. | рабочий набор макс. | ядер ср. | ядер макс. | каталог данных | WAL | RSS/серия | диск/серия |\n",
	"| active series | elapsed / planned | write request p50 | p99 | 2xx | 4xx | 5xx | client errors |\n":                                                                                     "| активных серий | прошло / план | запрос записи p50 | p99 | 2xx | 4xx | 5xx | ошибки клиента |\n",
	"\nStep %d errors: `%s`\n": "\nОшибки шага %d: `%s`\n",
	"### Comparison\n\n":       "### Сравнение\n\n",
	"Lower is better for every memory and cpu column. RSS is the engine's own process_resident_memory_bytes, working set is the cgroup figure the kubelet evicts and OOM kills on.\n\n": "Для всех столбцов с памятью и процессором меньше значит лучше. RSS это `process_resident_memory_bytes` самого движка, рабочий набор это значение cgroup, по которому kubelet выселяет поды и срабатывает OOM.\n\n",
	"| active series | engine | RSS avg | bytes/series | working set max | cpu cores avg | data dir | samples/s |\n|---|---|---|---|---|---|---|---|\n":                                 "| активных серий | движок | RSS ср. | байт на серию | рабочий набор макс. | ядер ср. | каталог данных | точек/с |\n|---|---|---|---|---|---|---|---|\n",
	"At the largest step, `%s` needs the least resident memory per series: %s\n\n":                                                                                                      "На самой большой ступени меньше всего памяти на серию нужно `%s`: %s\n\n",
	"## Query latency\n\n": "## Время запросов\n\n",
	"Each query runs on its own: one unmeasured warmup request, then the given number of workers repeat it until the time budget is spent and a minimum number of requests finished. ": "Каждый запрос идёт отдельно: один пробный запрос без учёта, затем заданное число потоков повторяет его, пока не истечёт отведённое время и не наберётся минимальное число запросов. ",
	"`n` is the number of measured requests. With a small `n` the p99 is close to the maximum, so the median is the figure to compare. ":                                               "`n` это число учтённых запросов. При малом `n` значение p99 близко к максимуму, поэтому сравнивать лучше медиану. ",
	"The geometric mean weighs every query equally, so a few multi second range queries do not drown the rest.\n\n":                                                                    "Геометрическое среднее учитывает все запросы поровну, поэтому несколько запросов, которые идут секундами, не заглушают остальные.\n\n",
	"![latency under concurrency](charts/latency-scaling.svg)\n\n":                                                                                                                     "![время запросов при разной параллельности](charts/latency-scaling.svg)\n\n",
	"### Concurrency %d\n\n":                                 "### Параллельность %d\n\n",
	"![median query latency, concurrency %d](charts/%s)\n\n": "![медианное время запросов, параллельность %d](charts/%s)\n\n",
	"| engine | suite | queries | requests | errors | geomean p50 | geomean p99 | slowest query p50 | cpu cores avg | working set max |\n": "| движок | набор | запросов | замеров | ошибок | p50 (геом.) | p99 (геом.) | p50 самого медленного | ядер ср. | рабочий набор макс. |\n",
	"## Result equality\n\n": "## Совпадение результатов\n\n",
	"Every engine received the same samples and every query is evaluated at the same pinned timestamp, so a result that differs points at a semantic difference between engines or at lost data. Each cell is the sha256 of the canonicalised result.\n\n":                                                                                                                                                                                                                "Все движки получили одни и те же точки, а каждый запрос вычисляется в один и тот же закреплённый момент, так что отличающийся результат указывает на смысловое различие между движками или на потерянные данные. В каждой ячейке sha256 результата, приведённого к единому виду.\n\n",
	"A differing row means the engines disagree on the same data at the same timestamp. All of them are rate or `_over_time` range queries. Prometheus 3.0 made range selectors left-open, a sample exactly on the window start is no longer included, and the Prom++ 0.8.15 PromQL engine already drops it (`promql/engine.go`, `floats[drop].T <= mint`), unlike 2.55.1 (`< mint`). The synthetic samples sit on the window edge, so 2.55.1 sees one extra sample.\n\n": "Строка с различием значит, что движки по-разному отвечают на одни и те же данные в один и тот же момент. Все такие запросы это `rate` или `_over_time` по диапазону. В Prometheus 3.0 диапазон стал открытым слева: точка ровно на левой границе окна больше не входит в расчёт. PromQL-движок Prom++ 0.8.15 её уже отбрасывает (`promql/engine.go`, `floats[drop].T <= mint`), а 2.55.1 нет (`< mint`). Искусственные точки лежат ровно на границе окна, поэтому 2.55.1 видит на одну точку больше.\n\n",
	"| query |":           "| запрос |",
	" identical |\n|---|": " совпадает |\n|---|",
	" yes |\n":            " да |\n", " no |\n": " нет |\n", " error |": " ошибка |",
	"\n%d queries identical, %d differ.\n\n": "\nСовпадают %d запросов, различаются %d.\n\n",
	"## Reproduce\n\n":                       "## Как воспроизвести\n\n",
	"export BENCH_NODE=<node>\n":             "export BENCH_NODE=<узел>\n",
	"See METHODOLOGY.md for the fairness rules and the known threats to validity.\n": "Правила честного сравнения и то, что может исказить результат, описаны в METHODOLOGY.ru.md.\n",
}
