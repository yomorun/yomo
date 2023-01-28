# Optimizations & Best Practices

## Data Codec

**JSON** (**JavaScript Object Notation**, pronounced [/ˈdʒeɪsən/](https://en.wikipedia.org/wiki/Help:IPA/English); also [/ˈdʒeɪˌsɒn/](https://en.wikipedia.org/wiki/Help:IPA/English)) is an [open standard](https://en.wikipedia.org/wiki/Open_standard) [file format](https://en.wikipedia.org/wiki/File_format) and [data interchange](https://en.wikipedia.org/wiki/Electronic_data_interchange) format that uses [human-readable](https://en.wikipedia.org/wiki/Human-readable_medium) text to store and transmit data objects consisting of [attribute–value pairs](https://en.wikipedia.org/wiki/Attribute–value_pair) and [arrays](https://en.wikipedia.org/wiki/Array_data_type) (or other [serializable](https://en.wikipedia.org/wiki/Serialization) values). In the example, a large number of data formats are used to transmit data streams, but the encoding and decoding efficiency in the production environment is low and the size is large. In order to ensure the efficient transmission of stream data, it is recommended that you use binary encoding formats such as  [Y3](https://github.com/yomorun/y3) , [MessagePack](https://msgpack.org/) , [ProtocolBuffers](https://developers.google.com/protocol-buffers/) .

## Security

`YoMo` supports in-transit encryption of communications between `Zipper`, `Source`, `StreamFucntion` using a central Certificate Authority(CA) .

`YoMo` allows operators and developers to bring in their own certificates, the `scripts` directory provides certificate generation scripts:

- generate_ca.sh
- generate_client.sh
- generate_server.sh

You can read it in the [README.md](https://github.com/yomorun/yomo/blob/master/scripts/README.md) file to create the relevant certificate.

By default, we use the `development` development mode and do not perform mutual `TLS` authentication between the server and the client. In a production environment, it is **strongly recommended** you modify the following environment variables:

- `YOMO_TLS_VERIFY_PEER`, Set the value to `true`
- `YOMO_TLS_CACERT_FILE`, CA certificate
- `YOMO_TLS_CERT_FILE`, Certificate
- `YOMO_TLS_KEY_FILE`, Private Key

In `Zipper`, `Source` the `StreamFucntion` instance configures the corresponding certificate file respectively.

Refer to Example [3-multi-sfn run settings](https://github.com/yomorun/yomo/blob/master/example/3-multi-sfn/Taskfile.yml) and uncomment some of the settings.
