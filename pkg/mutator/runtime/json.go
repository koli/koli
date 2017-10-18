package runtime

import (
	"encoding/json"
	"io"

	"github.com/ghodss/yaml"
	"github.com/golang/glog"
	"github.com/ugorji/go/codec"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	runtimejson "k8s.io/apimachinery/pkg/runtime/serializer/json"
	"k8s.io/apimachinery/pkg/runtime/serializer/recognizer"
	"k8s.io/client-go/kubernetes/scheme"
)

var jsonSerializerInfo = serializerInfoOrDie()

type Serializer struct {
	meta    runtimejson.MetaFactory
	creater runtime.ObjectCreater
	typer   runtime.ObjectTyper
	yaml    bool
	pretty  bool
}

// NewSerializer creates a JSON serializer that handles encoding versioned objects into the proper JSON form. If typer
// is not nil, the object has the group, version, and kind fields set.
func NewSerializer(meta runtimejson.MetaFactory, creater runtime.ObjectCreater, typer runtime.ObjectTyper, pretty bool) *Serializer {
	return &Serializer{
		meta:    meta,
		creater: creater,
		typer:   typer,
		yaml:    false,
		pretty:  pretty,
	}
}

// NewYAMLSerializer creates a YAML serializer that handles encoding versioned objects into the proper YAML form. If typer
// is not nil, the object has the group, version, and kind fields set. This serializer supports only the subset of YAML that
// matches JSON, and will error if constructs are used that do not serialize to JSON.
func NewYAMLSerializer(meta runtimejson.MetaFactory, creater runtime.ObjectCreater, typer runtime.ObjectTyper) *Serializer {
	return &Serializer{
		meta:    meta,
		creater: creater,
		typer:   typer,
		yaml:    true,
	}
}

// // serializerExtensions are for serializers that are conditionally compiled in
var serializerExtensions = []func(*runtime.Scheme) (serializerType, bool){}

type serializerType struct {
	AcceptContentTypes []string
	ContentType        string
	FileExtensions     []string
	// EncodesAsText should be true if this content type can be represented safely in UTF-8
	EncodesAsText bool

	Serializer       runtime.Serializer
	PrettySerializer runtime.Serializer

	AcceptStreamContentTypes []string
	StreamContentType        string

	Framer           runtime.Framer
	StreamSerializer runtime.Serializer
}

func newSerializersForScheme(scheme *runtime.Scheme, mf runtimejson.MetaFactory) []serializerType {
	Serializer := NewSerializer(mf, scheme, scheme, false)
	jsonPrettySerializer := NewSerializer(mf, scheme, scheme, true)
	yamlSerializer := NewYAMLSerializer(mf, scheme, scheme)

	serializers := []serializerType{
		{
			AcceptContentTypes: []string{"application/json"},
			ContentType:        "application/json",
			FileExtensions:     []string{"json"},
			EncodesAsText:      true,
			Serializer:         Serializer,
			PrettySerializer:   jsonPrettySerializer,

			Framer:           runtimejson.Framer,
			StreamSerializer: Serializer,
		},
		{
			AcceptContentTypes: []string{"application/yaml"},
			ContentType:        "application/yaml",
			FileExtensions:     []string{"yaml"},
			EncodesAsText:      true,
			Serializer:         yamlSerializer,
		},
	}

	for _, fn := range serializerExtensions {
		if serializer, ok := fn(scheme); ok {
			serializers = append(serializers, serializer)
		}
	}
	return serializers
}

// Decode attempts to convert the provided data into YAML or JSON, extract the stored schema kind, apply the provided default gvk, and then
// load that data into an object matching the desired schema kind or the provided into. If into is *runtime.Unknown, the raw data will be
// extracted and no decoding will be performed. If into is not registered with the typer, then the object will be straight decoded using
// normal JSON/YAML unmarshalling. If into is provided and the original data is not fully qualified with kind/version/group, the type of
// the into will be used to alter the returned gvk. On success or most errors, the method will return the calculated schema kind.
func (s *Serializer) Decode(originalData []byte, gvk *schema.GroupVersionKind, into runtime.Object) (runtime.Object, *schema.GroupVersionKind, error) {
	if versioned, ok := into.(*runtime.VersionedObjects); ok {
		into = versioned.Last()
		obj, actual, err := s.Decode(originalData, gvk, into)
		if err != nil {
			return nil, actual, err
		}
		versioned.Objects = []runtime.Object{obj}
		return versioned, actual, nil
	}

	data := originalData
	if s.yaml {
		altered, err := yaml.YAMLToJSON(data)
		if err != nil {
			return nil, nil, err
		}
		data = altered
	}

	actual, err := s.meta.Interpret(data)
	if err != nil {
		return nil, nil, err
	}

	if gvk != nil {
		// apply kind and version defaulting from provided default
		if len(actual.Kind) == 0 {
			actual.Kind = gvk.Kind
		}
		if len(actual.Version) == 0 && len(actual.Group) == 0 {
			actual.Group = gvk.Group
			actual.Version = gvk.Version
		}
		if len(actual.Version) == 0 && actual.Group == gvk.Group {
			actual.Version = gvk.Version
		}
	}

	if unk, ok := into.(*runtime.Unknown); ok && unk != nil {
		unk.Raw = originalData
		unk.ContentType = runtime.ContentTypeJSON
		unk.GetObjectKind().SetGroupVersionKind(*actual)
		return unk, actual, nil
	}

	if into != nil {
		types, _, err := s.typer.ObjectKinds(into)
		switch {
		case runtime.IsNotRegisteredError(err):
			if err := codec.NewDecoderBytes(data, new(codec.JsonHandle)).Decode(into); err != nil {
				return nil, actual, err
			}
			return into, actual, nil
		case err != nil:
			return nil, actual, err
		default:
			typed := types[0]
			if len(actual.Kind) == 0 {
				actual.Kind = typed.Kind
			}
			if len(actual.Version) == 0 && len(actual.Group) == 0 {
				actual.Group = typed.Group
				actual.Version = typed.Version
			}
			if len(actual.Version) == 0 && actual.Group == typed.Group {
				actual.Version = typed.Version
			}
		}
	}

	if len(actual.Kind) == 0 {
		return nil, actual, runtime.NewMissingKindErr(string(originalData))
	}
	if len(actual.Version) == 0 {
		return nil, actual, runtime.NewMissingVersionErr(string(originalData))
	}

	// use the target if necessary
	obj, err := runtime.UseOrCreateObject(s.typer, s.creater, *actual, into)
	if err != nil {
		return nil, actual, err
	}

	if err := codec.NewDecoderBytes(data, new(codec.JsonHandle)).Decode(obj); err != nil {
		return nil, actual, err
	}
	return obj, actual, nil
}

// Encode serializes the provided object to the given writer.
func (s *Serializer) Encode(obj runtime.Object, w io.Writer) error {
	if s.yaml {
		json, err := json.Marshal(obj)
		if err != nil {
			return err
		}
		data, err := yaml.JSONToYAML(json)
		if err != nil {
			return err
		}
		_, err = w.Write(data)
		return err
	}

	if s.pretty {
		data, err := json.MarshalIndent(obj, "", "  ")
		if err != nil {
			return err
		}
		_, err = w.Write(data)
		return err
	}
	encoder := json.NewEncoder(w)
	return encoder.Encode(obj)
}

func serializerInfoOrDie() runtime.SerializerInfo {
	jsonSerializerInfo, ok := runtime.SerializerInfoForMediaType(scheme.Codecs.SupportedMediaTypes(), runtime.ContentTypeJSON)
	if !ok {
		glog.Fatalf(`No serializer for "%s"`, runtime.ContentTypeJSON)
	}
	return jsonSerializerInfo
}

// LegacyCodec it's a codec which uses ugorji instead of jsoniter, mutator has some bugs
// when dealing with encoding/decoding of common types. We expect to Deprecate mutator in the
// future and use webhooks to mutate or deny/allow access to attributes
func LegacyCodec(versions ...schema.GroupVersion) runtime.Codec {
	serializers := newSerializersForScheme(scheme.Scheme, runtimejson.DefaultMetaFactory)
	decoders := make([]runtime.Decoder, 0, len(serializers))
	for _, d := range serializers {
		decoders = append(decoders, d.Serializer)
	}
	return scheme.Codecs.CodecForVersions(
		jsonSerializerInfo.Serializer,
		recognizer.NewDecoder(decoders...),
		schema.GroupVersions(versions),
		runtime.InternalGroupVersioner,
	)
}
