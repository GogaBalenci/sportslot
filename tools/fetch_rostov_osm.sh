#!/usr/bin/env sh
# Выгрузка спортобъектов Ростова-на-Дону из OpenStreetMap (Overpass API).
# Результат: rostov_osm.json рядом со скриптом.
# Overpass отклоняет запросы без понятного User-Agent (HTTP 406), поэтому он задан явно.
# Прямоугольник захватывает окраины Батайска и Аксая — лишнее отсекает импортёр.
set -eu
cd "$(dirname "$0")"

UA="SportSlot/1.0 (+https://sportslot.space)"
BBOX="47.14,39.45,47.37,39.87"

QUERY="[out:json][timeout:120][bbox:$BBOX];
(
  nwr[\"leisure\"~\"^(fitness_centre|sports_centre|sports_hall|swimming_pool|dance|ice_rink)$\"];
  nwr[\"amenity\"=\"dojo\"];
  nwr[\"club\"=\"sport\"];
  nwr[\"sport\"][\"name\"];
);
out center tags;"

for API in \
  https://overpass-api.de/api/interpreter \
  https://overpass.kumi.systems/api/interpreter \
  https://lz4.overpass-api.de/api/interpreter
do
  echo "Запрос к $API ..."
  CODE=$(curl -sS --connect-timeout 15 --max-time 180 \
           -A "$UA" -H "Accept: application/json" \
           --data-urlencode "data=$QUERY" "$API" \
           -o rostov_osm.json.tmp -w '%{http_code}' || echo 000)
  COUNT=$(grep -o '"type": *"\(node\|way\|relation\)"' rostov_osm.json.tmp 2>/dev/null | wc -l | tr -d ' ')
  if [ "$CODE" = "200" ] && [ "${COUNT:-0}" -gt 0 ]; then
    mv rostov_osm.json.tmp rostov_osm.json
    echo "Готово: $COUNT объектов, файл $(pwd)/rostov_osm.json"
    exit 0
  fi
  echo "HTTP $CODE, объектов: ${COUNT:-0}. Начало ответа:"
  head -c 400 rostov_osm.json.tmp 2>/dev/null; echo; echo
done

rm -f rostov_osm.json.tmp
echo "Не удалось получить данные. Пришлите вывод выше." >&2
exit 1
