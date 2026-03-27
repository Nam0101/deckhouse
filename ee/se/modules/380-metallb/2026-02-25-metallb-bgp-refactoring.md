# ADR: Рефакторинг BGP-конфигурации MetalLB и внедрение модульной архитектуры CRD

## Описание

Данный документ описывает изменение архитектуры конфигурации балансировщиков MetalLB в Deckhouse для **BGP-режима**.

Мы отказываемся от настройки BGP через глобальный `ModuleConfig` в пользу модульного подхода с разделением сущностей (Pool, Peer, Configuration) через отдельные CRD, а также стандартизируем привязку сервисов через аннотации.

### Контекст

- [Оригинальная документация BGP конфигурации MetalLB](https://metallb.io/configuration/_advanced_bgp_configuration/)
- Текущая реализация режима **BGP** в Deckhouse использует практически оригинальную реализацию MetalLB и настраивается глобально через единый объект `ModuleConfig`.

## Мотивация / Боль

1. **Монолитность и негибкость ModuleConfig:** Вся конфигурация BGP (пиры, пулы, параметры анонсирования) хранится в одном глобальном конфиге модуля. Это делает невозможным гранулярное управление конфигурациями для разных групп узлов без раздувания и усложнения структуры `ModuleConfig`.
2. **Отказ от использования `spec.loadBalancerClass`:** В отличие от текущей реализации L2-режима, мы осознанно не внедряем штатное поле Kubernetes `spec.loadBalancerClass` для привязки BGP-пулов. Данное поле является иммутабельным (неизменяемым), что приводит к невозможности изменения пула адресов у существующего сервиса "на лету" без его полного пересоздания.
3. **Отсутствие переиспользования:** Невозможно элегантно переиспользовать одни и те же пулы или настройки роутеров-соседей для разных политик маршрутизации.

## Область

Данный документ затрагивает механизмы настройки **только BGP-маршрутизации** и выделения IP-адресов для BGP-сервисов в кластерах с использованием модуля `metallb`.

### Цели

- Вынести конфигурацию BGP из `ModuleConfig`.
- Внедрить новые модульные CRD: `MetalLoadBalancerPool`, `MetalLoadBalancerBGPPeer`, `MetalLoadBalancerConfiguration` для управления BGP-режимом.
- Использовать аннотацию `network.deckhouse.io/load-balancer-pool` для привязки сервисов к пулам.

### Не цели

- Рефакторинг конфигурации L2-режима и изменение ресурса `MetalLoadBalancerClass`.

## Детальное описание решения

### 1. Архитектурное разделение сущностей

Для конфигурации BGP вводятся три независимых CRD:

- **`MetalLoadBalancerPool`**: Хранит только списки IP-адресов. Это изолированная логическая единица выделения адресов.
- **`MetalLoadBalancerBGPPeer`**: Описывает настройки BGP-соседа (роутера). Ресурс независим и может переиспользоваться.
- **`MetalLoadBalancerConfiguration`**: Логический "клей", который задает правила анонса: с каких узлов (`nodeSelector`), каким пирам и какие пулы анонсировать, включая специфичные параметры BGP (communities, localPref).

### 2. Привязка сервисов к пулу

Для привязки сервиса к конкретному BGP-пулу адресов будет использоваться аннотация `network.deckhouse.io/load-balancer-pool`. Использование аннотации позволяет изменять пул адресов у `Service` "на лету" без необходимости его удаления, так как аннотации, в отличие от поля `spec.loadBalancerClass`, являются мутабельными.

```yaml
kind: Service
metadata:
  name: my_svc
  annotations:
    network.deckhouse.io/load-balancer-pool: prod-ips
spec:
  type: LoadBalancer
  ports:
    - port: 80
      targetPort: 8080
```

### 3. Структура новых API ресурсов

#### MetalLoadBalancerPool

Независимый ресурс, представляющий собой список IP-адресов.

```yaml
apiVersion: network.deckhouse.io/v1alpha1
kind: MetalLoadBalancerPool
metadata:
  name: prod-ips
spec:
  addresses:
    - 198.51.100.0/24
    - 198.51.101.0/24
```

#### MetalLoadBalancerBGPPeer

Независимый ресурс, описывающий BGP-маршрутизатор (соседа) и параметры установки сессии с ним (включая локальные loopback-адреса узлов).

```yaml
apiVersion: network.deckhouse.io/v1alpha1
kind: MetalLoadBalancerBGPPeer
metadata:
  name: tor-router-1
spec:
  peerAddress: 10.0.0.32
  peerPort: 179
  peerASN: 64503
  myASN: 64600
  holdTime: 3s
  passwordSecretRef:
    name: router-password
    namespace: my-namespace
  # Поузловое переопределение локального адреса для пиринга (например, для loopback)
  sourceAddresses:
    - nodeName: front-1
      address: 10.10.10.1
    - nodeName: front-2
      address: 10.10.10.2
  # Параметры BFD инкапсулируются прямо в пира для удобства
  bfd:
    receiveInterval: 300
    transmitInterval: 300
    detectMultiplier: 3
    echoInterval: 50
    echoMode: false
    passiveMode: false
    minimumTtl: 254
```

#### MetalLoadBalancerConfiguration

Ресурс, определяющий топологию маршрутизации (для BGP). Связывает узлы кластера с роутерами и пулами IP-адресов.

```yaml
apiVersion: network.deckhouse.io/v1alpha1
kind: MetalLoadBalancerConfiguration
metadata:
  name: frontend-bgp-routing
spec:
  # 1. ГДЕ: с каких узлов анонсируем
  nodeSelector:
    node-role.deckhouse.io/frontend: ""

  # 2. КАК: тип маршрутизации
  mode: BGP

  # 3. КОМУ: ссылки на заранее созданных пиров
  bgp:
    peerNames:
      - tor-router-1
      - tor-router-2

  # 4. ЧТО: список пулов и настройки их анонса для этой конфигурации
  advertisements:
    - poolNames: ["prod-ips"]
      bgp:
        communities: ["42:300"]
        localPref: 111
        aggregationLength: 24
```

### 4. Обновленный ModuleConfig (очистка BGP)

Конфигурация самого модуля (`ModuleConfig`) будет полностью очищена от параметров BGP (`bgpPeers`, `addressPools`, `bgpCommunities`). В итоге ресурс `ModuleConfig` для `metallb` будет фактически пустым.

```yaml
apiVersion: deckhouse.io/v1alpha1
kind: ModuleConfig
metadata:
  name: metallb
spec:
  version: 3
  enable: true
```

### Имплементация

1. **API:** Добавление OpenAPI схем для новых ресурсов `MetalLoadBalancerPool`, `MetalLoadBalancerBGPPeer`, `MetalLoadBalancerConfiguration`.
2. **Генерация ресурсов:** Разработка хуков для преобразования новых сущностей во внутренние ресурсы, на основе которых Helm формирует оригинальные CRD MetalLB для BGP (`IPAddressPool`, `BGPPeer`, `BGPAdvertisement`). Особенности преобразования:
   - Секреты для BGP-паролей (`passwordSecretRef`), указанные пользователем в произвольных неймспейсах, будут автоматически копироваться хуками в системный неймспейс `d8-metallb` (так как оригинальный MetalLB требует нахождения секрета строго в своем неймспейсе).
   - Вложенная структура `bfd` из нашего `MetalLoadBalancerBGPPeer` будет "на лету" транслироваться в скрытые оригинальные CRD `BFDProfile`, а в итоговом объекте `BGPPeer` проставится соответствующая ссылка `bfdProfile`. При этом BFD будет работать только при использовании FRR-бэкенда.
3. **Миграция:** Автоматическая конвертация `ModuleConfig` с выносом старых настроек BGP в новые ресурсы во время обновления платформы.
4. **Аккумуляция селекторов узлов:** Для упрощения логики хук будет избегать сложных выражений:
   - **Для BGPPeer:** Хук читает список узлов из `sourceAddresses` (в ресурсе `MetalLoadBalancerBGPPeer`) и сопоставляет их с глобальным `nodeSelector` (из `MetalLoadBalancerConfiguration`, где указан этот пир). Если узел явно указан в `sourceAddresses`, для него генерируется персональный `BGPPeer` с жесткой привязкой (`matchLabels: {kubernetes.io/hostname: <nodeName>}`) и уникальным IP. Для остальных узлов генерируется общий "fallback" `BGPPeer` на основе глобального `nodeSelector` (без указания `sourceAddress`).
   - **Для BGPAdvertisement:** Глобальный `nodeSelector` из конфигурации переносится напрямую в сгенерированные анонсы пулов (`matchLabels`).
   - **Для Speaker DaemonSet:** Так как явный селектор узлов удален из `ModuleConfig`, хук динамически собирает все `nodeSelector` из всех конфигураций (плюс явно указанные узлы из пиров). Поскольку штатное поле `nodeSelector` у подов работает только как логическое И (AND), собранные селекторы преобразуются в конструкцию `nodeAffinity` (а именно `nodeSelectorTerms`, где каждый элемент работает как логическое ИЛИ). Это позволяет прозрачно разворачивать спикеры на всех необходимых группах узлов. Если маршрутизирующих конфигураций нет, применяется системный дефолтный селектор (`node-role.deckhouse.io/frontend: ""`).

### Пользовательский опыт

Пользователь получает четкое разделение абстракций для настройки BGP:

- Ресурсы IP -> `MetalLoadBalancerPool`.
- Сетевые роутеры -> `MetalLoadBalancerBGPPeer`.
- Правила маршрутизации -> `MetalLoadBalancerConfiguration`.

Использование изменяемой аннотации `network.deckhouse.io/load-balancer-pool` решает проблемы с необходимостью удаления сервисов при смене пула.

## Минусы внедрения решения

- **Увеличение количества сущностей:** Вместо одного раздела в `ModuleConfig` пользователю придется создать несколько манифестов CRD. Порог входа для базовой настройки незначительно возрастает.

## Рассмотренные альтернативы

- **Использование штатного `spec.loadBalancerClass`:** Отвергнуто из-за технической невозможности изменять это поле у существующего объекта `Service`. Обход этого ограничения (как это сделано в текущей реализации L2-режима) требует генерации подкапотных "теневых" сервисов, что значительно усложняет архитектуру и администрирование кластера.
- **Сохранение конфигурации в `ModuleConfig`:** Отвергнуто, так как это ведет к чрезмерному разрастанию структуры конфига и невозможности гибкой настройки для разных групп узлов.

## Ответственные контактные лица

- Андрей Половов <andrey.polovov@flant.com>
- Андрей Павлов <andrey.pavlov@flant.com>
- Денис Тарабрин <denis.tarabrin@flant.com>
