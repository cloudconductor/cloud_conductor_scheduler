MetronomeはConsulクラスタ上に構成されたサーバ間で順序制御を行うツールです。

複数のサーバで構成されたシステムにおいては、処理の順番が重要になることがあります。たとえば、データベースサーバはアプリケーションサーバよりも先に初期化を終えていなくてはなりませんし、ロードバランサーは全てのサーバが準備できてからクライアントのリクエストを受け付けるべきでしょう。

Metronomeは設定ファイルとConsul KVSを用いることで、これらの順序制御を行うことができます。

![Metronomeのアーキテクチャ](https://raw.githubusercontent.com/wiki/cloudconductor/metronome/ja/diagram.png)

Consulイベントを受信すると、Consulのイベントハンドラによってmetronome pushが起動され、Consul KVS上のイベントキューに受信したイベントが追加されます。
Metronomeはイベントキューからイベントをポーリングし、設定ファイルで指定されたサービス名、タグ名を条件として追加した上で実行タスクキューにイベントを格納します。
各サーバは実行タスクキューの先頭タスクを取得し、条件を満たした場合には処理を行います。他のサーバは先頭タスクを取得しながらこれらの処理が終了するのを待機します。

- [User Manual(ja)](https://github.com/cloudconductor/metronome/wiki/User-Manual(ja))
- [Scheduling file format(ja)](https://github.com/cloudconductor/metronome/wiki/Scheduling-file-format(ja))
