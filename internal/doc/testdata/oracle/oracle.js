(() => {
  var __defProp = Object.defineProperty;
  var __getOwnPropSymbols = Object.getOwnPropertySymbols;
  var __hasOwnProp = Object.prototype.hasOwnProperty;
  var __propIsEnum = Object.prototype.propertyIsEnumerable;
  var __defNormalProp = (obj, key, value) => key in obj ? __defProp(obj, key, { enumerable: true, configurable: true, writable: true, value }) : obj[key] = value;
  var __spreadValues = (a, b) => {
    for (var prop in b || (b = {}))
      if (__hasOwnProp.call(b, prop))
        __defNormalProp(a, prop, b[prop]);
    if (__getOwnPropSymbols)
      for (var prop of __getOwnPropSymbols(b)) {
        if (__propIsEnum.call(b, prop))
          __defNormalProp(a, prop, b[prop]);
      }
    return a;
  };
  var __objRest = (source, exclude) => {
    var target = {};
    for (var prop in source)
      if (__hasOwnProp.call(source, prop) && exclude.indexOf(prop) < 0)
        target[prop] = source[prop];
    if (source != null && __getOwnPropSymbols)
      for (var prop of __getOwnPropSymbols(source)) {
        if (exclude.indexOf(prop) < 0 && __propIsEnum.call(source, prop))
          target[prop] = source[prop];
      }
    return target;
  };

  // node_modules/orderedmap/dist/index.js
  function OrderedMap(content) {
    this.content = content;
  }
  OrderedMap.prototype = {
    constructor: OrderedMap,
    find: function(key) {
      for (var i = 0; i < this.content.length; i += 2)
        if (this.content[i] === key) return i;
      return -1;
    },
    // :: (string) → ?any
    // Retrieve the value stored under `key`, or return undefined when
    // no such key exists.
    get: function(key) {
      var found2 = this.find(key);
      return found2 == -1 ? void 0 : this.content[found2 + 1];
    },
    // :: (string, any, ?string) → OrderedMap
    // Create a new map by replacing the value of `key` with a new
    // value, or adding a binding to the end of the map. If `newKey` is
    // given, the key of the binding will be replaced with that key.
    update: function(key, value, newKey) {
      var self = newKey && newKey != key ? this.remove(newKey) : this;
      var found2 = self.find(key), content = self.content.slice();
      if (found2 == -1) {
        content.push(newKey || key, value);
      } else {
        content[found2 + 1] = value;
        if (newKey) content[found2] = newKey;
      }
      return new OrderedMap(content);
    },
    // :: (string) → OrderedMap
    // Return a map with the given key removed, if it existed.
    remove: function(key) {
      var found2 = this.find(key);
      if (found2 == -1) return this;
      var content = this.content.slice();
      content.splice(found2, 2);
      return new OrderedMap(content);
    },
    // :: (string, any) → OrderedMap
    // Add a new key to the start of the map.
    addToStart: function(key, value) {
      return new OrderedMap([key, value].concat(this.remove(key).content));
    },
    // :: (string, any) → OrderedMap
    // Add a new key to the end of the map.
    addToEnd: function(key, value) {
      var content = this.remove(key).content.slice();
      content.push(key, value);
      return new OrderedMap(content);
    },
    // :: (string, string, any) → OrderedMap
    // Add a key after the given key. If `place` is not found, the new
    // key is added to the end.
    addBefore: function(place, key, value) {
      var without = this.remove(key), content = without.content.slice();
      var found2 = without.find(place);
      content.splice(found2 == -1 ? content.length : found2, 0, key, value);
      return new OrderedMap(content);
    },
    // :: ((key: string, value: any))
    // Call the given function for each key/value pair in the map, in
    // order.
    forEach: function(f) {
      for (var i = 0; i < this.content.length; i += 2)
        f(this.content[i], this.content[i + 1]);
    },
    // :: (union<Object, OrderedMap>) → OrderedMap
    // Create a new map by prepending the keys in this map that don't
    // appear in `map` before the keys in `map`.
    prepend: function(map) {
      map = OrderedMap.from(map);
      if (!map.size) return this;
      return new OrderedMap(map.content.concat(this.subtract(map).content));
    },
    // :: (union<Object, OrderedMap>) → OrderedMap
    // Create a new map by appending the keys in this map that don't
    // appear in `map` after the keys in `map`.
    append: function(map) {
      map = OrderedMap.from(map);
      if (!map.size) return this;
      return new OrderedMap(this.subtract(map).content.concat(map.content));
    },
    // :: (union<Object, OrderedMap>) → OrderedMap
    // Create a map containing all the keys in this map that don't
    // appear in `map`.
    subtract: function(map) {
      var result = this;
      map = OrderedMap.from(map);
      for (var i = 0; i < map.content.length; i += 2)
        result = result.remove(map.content[i]);
      return result;
    },
    // :: () → Object
    // Turn ordered map into a plain object.
    toObject: function() {
      var result = {};
      this.forEach(function(key, value) {
        result[key] = value;
      });
      return result;
    },
    // :: number
    // The amount of keys in this map.
    get size() {
      return this.content.length >> 1;
    }
  };
  OrderedMap.from = function(value) {
    if (value instanceof OrderedMap) return value;
    var content = [];
    if (value) for (var prop in value) content.push(prop, value[prop]);
    return new OrderedMap(content);
  };
  var dist_default = OrderedMap;

  // node_modules/prosemirror-model/dist/index.js
  function findDiffStart(a, b, pos) {
    for (let i = 0; ; i++) {
      if (i == a.childCount || i == b.childCount)
        return a.childCount == b.childCount ? null : pos;
      let childA = a.child(i), childB = b.child(i);
      if (childA == childB) {
        pos += childA.nodeSize;
        continue;
      }
      if (!childA.sameMarkup(childB))
        return pos;
      if (childA.isText && childA.text != childB.text) {
        let tA = childA.text, tB = childB.text, j = 0;
        for (; tA[j] == tB[j]; j++)
          pos++;
        if (j && j < tA.length && j < tB.length && surrogateHigh(tA.charCodeAt(j - 1)) && surrogateLow(tA.charCodeAt(j)))
          pos--;
        return pos;
      }
      if (childA.content.size || childB.content.size) {
        let inner = findDiffStart(childA.content, childB.content, pos + 1);
        if (inner != null)
          return inner;
      }
      pos += childA.nodeSize;
    }
  }
  function findDiffEnd(a, b, posA, posB) {
    for (let iA = a.childCount, iB = b.childCount; ; ) {
      if (iA == 0 || iB == 0)
        return iA == iB ? null : { a: posA, b: posB };
      let childA = a.child(--iA), childB = b.child(--iB), size = childA.nodeSize;
      if (childA == childB) {
        posA -= size;
        posB -= size;
        continue;
      }
      if (!childA.sameMarkup(childB))
        return { a: posA, b: posB };
      if (childA.isText && childA.text != childB.text) {
        let tA = childA.text, tB = childB.text, iA2 = tA.length, iB2 = tB.length;
        while (iA2 > 0 && iB2 > 0 && tA[iA2 - 1] == tB[iB2 - 1]) {
          iA2--;
          iB2--;
          posA--;
          posB--;
        }
        if (iA2 && iB2 && iA2 < tA.length && surrogateHigh(tA.charCodeAt(iA2 - 1)) && surrogateLow(tA.charCodeAt(iA2))) {
          posA++;
          posB++;
        }
        return { a: posA, b: posB };
      }
      if (childA.content.size || childB.content.size) {
        let inner = findDiffEnd(childA.content, childB.content, posA - 1, posB - 1);
        if (inner)
          return inner;
      }
      posA -= size;
      posB -= size;
    }
  }
  function surrogateLow(ch) {
    return ch >= 56320 && ch < 57344;
  }
  function surrogateHigh(ch) {
    return ch >= 55296 && ch < 56320;
  }
  var Fragment = class _Fragment {
    /**
    @internal
    */
    constructor(content, size) {
      this.content = content;
      this.size = size || 0;
      if (size == null)
        for (let i = 0; i < content.length; i++)
          this.size += content[i].nodeSize;
    }
    /**
    Invoke a callback for all descendant nodes between the given two
    positions (relative to start of this fragment). Doesn't descend
    into a node when the callback returns `false`.
    */
    nodesBetween(from, to, f, nodeStart = 0, parent) {
      for (let i = 0, pos = 0; pos < to; i++) {
        let child = this.content[i], end = pos + child.nodeSize;
        if (end > from && f(child, nodeStart + pos, parent || null, i) !== false && child.content.size) {
          let start = pos + 1;
          child.nodesBetween(Math.max(0, from - start), Math.min(child.content.size, to - start), f, nodeStart + start);
        }
        pos = end;
      }
    }
    /**
    Call the given callback for every descendant node. `pos` will be
    relative to the start of the fragment. The callback may return
    `false` to prevent traversal of a given node's children.
    */
    descendants(f) {
      this.nodesBetween(0, this.size, f);
    }
    /**
    Extract the text between `from` and `to`. See the same method on
    [`Node`](https://prosemirror.net/docs/ref/#model.Node.textBetween).
    */
    textBetween(from, to, blockSeparator, leafText) {
      let text = "", first = true;
      this.nodesBetween(from, to, (node, pos) => {
        let nodeText = node.isText ? node.text.slice(Math.max(from, pos) - pos, to - pos) : !node.isLeaf ? "" : leafText ? typeof leafText === "function" ? leafText(node) : leafText : node.type.spec.leafText ? node.type.spec.leafText(node) : "";
        if (node.isBlock && (node.isLeaf && nodeText || node.isTextblock) && blockSeparator) {
          if (first)
            first = false;
          else
            text += blockSeparator;
        }
        text += nodeText;
      }, 0);
      return text;
    }
    /**
    Create a new fragment containing the combined content of this
    fragment and the other.
    */
    append(other) {
      if (!other.size)
        return this;
      if (!this.size)
        return other;
      let last = this.lastChild, first = other.firstChild, content = this.content.slice(), i = 0;
      if (last.isText && last.sameMarkup(first)) {
        content[content.length - 1] = last.withText(last.text + first.text);
        i = 1;
      }
      for (; i < other.content.length; i++)
        content.push(other.content[i]);
      return new _Fragment(content, this.size + other.size);
    }
    /**
    Cut out the sub-fragment between the two given positions.
    */
    cut(from, to = this.size) {
      if (from == 0 && to == this.size)
        return this;
      let result = [], size = 0;
      if (to > from)
        for (let i = 0, pos = 0; pos < to; i++) {
          let child = this.content[i], end = pos + child.nodeSize;
          if (end > from) {
            if (pos < from || end > to) {
              if (child.isText)
                child = child.cut(Math.max(0, from - pos), Math.min(child.text.length, to - pos));
              else
                child = child.cut(Math.max(0, from - pos - 1), Math.min(child.content.size, to - pos - 1));
            }
            result.push(child);
            size += child.nodeSize;
          }
          pos = end;
        }
      return new _Fragment(result, size);
    }
    /**
    @internal
    */
    cutByIndex(from, to) {
      if (from == to)
        return _Fragment.empty;
      if (from == 0 && to == this.content.length)
        return this;
      return new _Fragment(this.content.slice(from, to));
    }
    /**
    Create a new fragment in which the node at the given index is
    replaced by the given node.
    */
    replaceChild(index, node) {
      let current = this.content[index];
      if (current == node)
        return this;
      let copy = this.content.slice();
      let size = this.size + node.nodeSize - current.nodeSize;
      copy[index] = node;
      return new _Fragment(copy, size);
    }
    /**
    Create a new fragment by prepending the given node to this
    fragment.
    */
    addToStart(node) {
      return new _Fragment([node].concat(this.content), this.size + node.nodeSize);
    }
    /**
    Create a new fragment by appending the given node to this
    fragment.
    */
    addToEnd(node) {
      return new _Fragment(this.content.concat(node), this.size + node.nodeSize);
    }
    /**
    Compare this fragment to another one.
    */
    eq(other) {
      if (this.content.length != other.content.length)
        return false;
      for (let i = 0; i < this.content.length; i++)
        if (!this.content[i].eq(other.content[i]))
          return false;
      return true;
    }
    /**
    The first child of the fragment, or `null` if it is empty.
    */
    get firstChild() {
      return this.content.length ? this.content[0] : null;
    }
    /**
    The last child of the fragment, or `null` if it is empty.
    */
    get lastChild() {
      return this.content.length ? this.content[this.content.length - 1] : null;
    }
    /**
    The number of child nodes in this fragment.
    */
    get childCount() {
      return this.content.length;
    }
    /**
    Get the child node at the given index. Raise an error when the
    index is out of range.
    */
    child(index) {
      let found2 = this.content[index];
      if (!found2)
        throw new RangeError("Index " + index + " out of range for " + this);
      return found2;
    }
    /**
    Get the child node at the given index, if it exists.
    */
    maybeChild(index) {
      return this.content[index] || null;
    }
    /**
    Call `f` for every child node, passing the node, its offset
    into this parent node, and its index.
    */
    forEach(f) {
      for (let i = 0, p = 0; i < this.content.length; i++) {
        let child = this.content[i];
        f(child, p, i);
        p += child.nodeSize;
      }
    }
    /**
    Find the first position at which this fragment and another
    fragment differ, or `null` if they are the same.
    */
    findDiffStart(other, pos = 0) {
      return findDiffStart(this, other, pos);
    }
    /**
    Find the first position, searching from the end, at which this
    fragment and the given fragment differ, or `null` if they are
    the same. Since this position will not be the same in both
    nodes, an object with two separate positions is returned.
    */
    findDiffEnd(other, pos = this.size, otherPos = other.size) {
      return findDiffEnd(this, other, pos, otherPos);
    }
    /**
    Find the index and inner offset corresponding to a given relative
    position in this fragment. The result object will be reused
    (overwritten) the next time the function is called. @internal
    */
    findIndex(pos) {
      if (pos == 0)
        return retIndex(0, pos);
      if (pos == this.size)
        return retIndex(this.content.length, pos);
      if (pos > this.size || pos < 0)
        throw new RangeError(`Position ${pos} outside of fragment (${this})`);
      for (let i = 0, curPos = 0; ; i++) {
        let cur = this.child(i), end = curPos + cur.nodeSize;
        if (end >= pos) {
          if (end == pos)
            return retIndex(i + 1, end);
          return retIndex(i, curPos);
        }
        curPos = end;
      }
    }
    /**
    Return a debugging string that describes this fragment.
    */
    toString() {
      return "<" + this.toStringInner() + ">";
    }
    /**
    @internal
    */
    toStringInner() {
      return this.content.join(", ");
    }
    /**
    Create a JSON-serializeable representation of this fragment.
    */
    toJSON() {
      return this.content.length ? this.content.map((n) => n.toJSON()) : null;
    }
    /**
    Deserialize a fragment from its JSON representation.
    */
    static fromJSON(schema2, value) {
      if (!value)
        return _Fragment.empty;
      if (!Array.isArray(value))
        throw new RangeError("Invalid input for Fragment.fromJSON");
      return _Fragment.fromArray(value.map(schema2.nodeFromJSON));
    }
    /**
    Build a fragment from an array of nodes. Ensures that adjacent
    text nodes with the same marks are joined together.
    */
    static fromArray(array) {
      if (!array.length)
        return _Fragment.empty;
      let joined, size = 0;
      for (let i = 0; i < array.length; i++) {
        let node = array[i];
        size += node.nodeSize;
        if (i && node.isText && array[i - 1].sameMarkup(node)) {
          if (!joined)
            joined = array.slice(0, i);
          joined[joined.length - 1] = node.withText(joined[joined.length - 1].text + node.text);
        } else if (joined) {
          joined.push(node);
        }
      }
      return new _Fragment(joined || array, size);
    }
    /**
    Create a fragment from something that can be interpreted as a
    set of nodes. For `null`, it returns the empty fragment. For a
    fragment, the fragment itself. For a node or array of nodes, a
    fragment containing those nodes.
    */
    static from(nodes) {
      if (!nodes)
        return _Fragment.empty;
      if (nodes instanceof _Fragment)
        return nodes;
      if (Array.isArray(nodes))
        return this.fromArray(nodes);
      if (nodes.attrs)
        return new _Fragment([nodes], nodes.nodeSize);
      throw new RangeError("Can not convert " + nodes + " to a Fragment" + (nodes.nodesBetween ? " (looks like multiple versions of prosemirror-model were loaded)" : ""));
    }
  };
  Fragment.empty = new Fragment([], 0);
  var found = { index: 0, offset: 0 };
  function retIndex(index, offset) {
    found.index = index;
    found.offset = offset;
    return found;
  }
  function compareDeep(a, b) {
    if (a === b)
      return true;
    if (!(a && typeof a == "object") || !(b && typeof b == "object"))
      return false;
    let array = Array.isArray(a);
    if (Array.isArray(b) != array)
      return false;
    if (array) {
      if (a.length != b.length)
        return false;
      for (let i = 0; i < a.length; i++)
        if (!compareDeep(a[i], b[i]))
          return false;
    } else {
      for (let p in a)
        if (!(p in b) || !compareDeep(a[p], b[p]))
          return false;
      for (let p in b)
        if (!(p in a))
          return false;
    }
    return true;
  }
  var Mark = class _Mark {
    /**
    @internal
    */
    constructor(type, attrs) {
      this.type = type;
      this.attrs = attrs;
    }
    /**
    Given a set of marks, create a new set which contains this one as
    well, in the right position. If this mark is already in the set,
    the set itself is returned. If any marks that are set to be
    [exclusive](https://prosemirror.net/docs/ref/#model.MarkSpec.excludes) with this mark are present,
    those are replaced by this one.
    */
    addToSet(set) {
      let copy, placed = false;
      for (let i = 0; i < set.length; i++) {
        let other = set[i];
        if (this.eq(other))
          return set;
        if (this.type.excludes(other.type)) {
          if (!copy)
            copy = set.slice(0, i);
        } else if (other.type.excludes(this.type)) {
          return set;
        } else {
          if (!placed && other.type.rank > this.type.rank) {
            if (!copy)
              copy = set.slice(0, i);
            copy.push(this);
            placed = true;
          }
          if (copy)
            copy.push(other);
        }
      }
      if (!copy)
        copy = set.slice();
      if (!placed)
        copy.push(this);
      return copy;
    }
    /**
    Remove this mark from the given set, returning a new set. If this
    mark is not in the set, the set itself is returned.
    */
    removeFromSet(set) {
      for (let i = 0; i < set.length; i++)
        if (this.eq(set[i]))
          return set.slice(0, i).concat(set.slice(i + 1));
      return set;
    }
    /**
    Test whether this mark is in the given set of marks.
    */
    isInSet(set) {
      for (let i = 0; i < set.length; i++)
        if (this.eq(set[i]))
          return true;
      return false;
    }
    /**
    Test whether this mark has the same type and attributes as
    another mark.
    */
    eq(other) {
      return this == other || this.type == other.type && compareDeep(this.attrs, other.attrs);
    }
    /**
    Convert this mark to a JSON-serializeable representation.
    */
    toJSON() {
      let obj = { type: this.type.name };
      for (let _ in this.attrs) {
        obj.attrs = this.attrs;
        break;
      }
      return obj;
    }
    /**
    Deserialize a mark from JSON.
    */
    static fromJSON(schema2, json) {
      if (!json)
        throw new RangeError("Invalid input for Mark.fromJSON");
      let type = schema2.marks[json.type];
      if (!type)
        throw new RangeError(`There is no mark type ${json.type} in this schema`);
      let mark = type.create(json.attrs);
      return mark;
    }
    /**
    Test whether two sets of marks are identical.
    */
    static sameSet(a, b) {
      if (a == b)
        return true;
      if (a.length != b.length)
        return false;
      for (let i = 0; i < a.length; i++)
        if (!a[i].eq(b[i]))
          return false;
      return true;
    }
    /**
    Create a properly sorted mark set from null, a single mark, or an
    unsorted array of marks.
    */
    static setFrom(marks) {
      if (!marks || Array.isArray(marks) && marks.length == 0)
        return _Mark.none;
      if (marks instanceof _Mark)
        return [marks];
      let copy = marks.slice();
      copy.sort((a, b) => a.type.rank - b.type.rank);
      return copy;
    }
  };
  Mark.none = [];
  var ReplaceError = class extends Error {
  };
  var Slice = class _Slice {
    /**
    Create a slice. When specifying a non-zero open depth, you must
    make sure that there are nodes of at least that depth at the
    appropriate side of the fragment—i.e. if the fragment is an
    empty paragraph node, `openStart` and `openEnd` can't be greater
    than 1.
    
    It is not necessary for the content of open nodes to conform to
    the schema's content constraints, though it should be a valid
    start/end/middle for such a node, depending on which sides are
    open.
    */
    constructor(content, openStart, openEnd) {
      this.content = content;
      this.openStart = openStart;
      this.openEnd = openEnd;
    }
    /**
    The size this slice would add when inserted into a document.
    */
    get size() {
      return this.content.size - this.openStart - this.openEnd;
    }
    /**
    @internal
    */
    insertAt(pos, fragment) {
      let content = insertInto(this.content, pos + this.openStart, fragment, this.openStart + 1, this.openEnd + 1);
      return content && new _Slice(content, this.openStart, this.openEnd);
    }
    /**
    @internal
    */
    removeBetween(from, to) {
      return new _Slice(removeRange(this.content, from + this.openStart, to + this.openStart), this.openStart, this.openEnd);
    }
    /**
    Tests whether this slice is equal to another slice.
    */
    eq(other) {
      return this.content.eq(other.content) && this.openStart == other.openStart && this.openEnd == other.openEnd;
    }
    /**
    @internal
    */
    toString() {
      return this.content + "(" + this.openStart + "," + this.openEnd + ")";
    }
    /**
    Convert a slice to a JSON-serializable representation.
    */
    toJSON() {
      if (!this.content.size)
        return null;
      let json = { content: this.content.toJSON() };
      if (this.openStart > 0)
        json.openStart = this.openStart;
      if (this.openEnd > 0)
        json.openEnd = this.openEnd;
      return json;
    }
    /**
    Deserialize a slice from its JSON representation.
    */
    static fromJSON(schema2, json) {
      if (!json)
        return _Slice.empty;
      let openStart = json.openStart || 0, openEnd = json.openEnd || 0;
      if (typeof openStart != "number" || typeof openEnd != "number")
        throw new RangeError("Invalid input for Slice.fromJSON");
      return new _Slice(Fragment.fromJSON(schema2, json.content), openStart, openEnd);
    }
    /**
    Create a slice from a fragment by taking the maximum possible
    open value on both side of the fragment.
    */
    static maxOpen(fragment, openIsolating = true) {
      let openStart = 0, openEnd = 0;
      for (let n = fragment.firstChild; n && !n.isLeaf && (openIsolating || !n.type.spec.isolating); n = n.firstChild)
        openStart++;
      for (let n = fragment.lastChild; n && !n.isLeaf && (openIsolating || !n.type.spec.isolating); n = n.lastChild)
        openEnd++;
      return new _Slice(fragment, openStart, openEnd);
    }
  };
  Slice.empty = new Slice(Fragment.empty, 0, 0);
  function removeRange(content, from, to) {
    let { index, offset } = content.findIndex(from), child = content.maybeChild(index);
    let { index: indexTo, offset: offsetTo } = content.findIndex(to);
    if (offset == from || child.isText) {
      if (offsetTo != to && !content.child(indexTo).isText)
        throw new RangeError("Removing non-flat range");
      return content.cut(0, from).append(content.cut(to));
    }
    if (index != indexTo)
      throw new RangeError("Removing non-flat range");
    return content.replaceChild(index, child.copy(removeRange(child.content, from - offset - 1, to - offset - 1)));
  }
  function insertInto(content, dist, insert, openStart, openEnd, parent) {
    let { index, offset } = content.findIndex(dist), child = content.maybeChild(index);
    if (offset == dist || child.isText) {
      if (parent && openStart <= 0 && openEnd <= 0 && !parent.canReplace(index, index, insert))
        return null;
      return content.cut(0, dist).append(insert).append(content.cut(dist));
    }
    let inner = insertInto(child.content, dist - offset - 1, insert, index == 0 ? openStart - 1 : 0, index == content.childCount - 1 ? openEnd - 1 : 0, child);
    return inner && content.replaceChild(index, child.copy(inner));
  }
  function replace($from, $to, slice) {
    if (slice.openStart > $from.depth)
      throw new ReplaceError("Inserted content deeper than insertion position");
    if ($from.depth - slice.openStart != $to.depth - slice.openEnd)
      throw new ReplaceError("Inconsistent open depths");
    return replaceOuter($from, $to, slice, 0);
  }
  function replaceOuter($from, $to, slice, depth) {
    let index = $from.index(depth), node = $from.node(depth);
    if (index == $to.index(depth) && depth < $from.depth - slice.openStart) {
      let inner = replaceOuter($from, $to, slice, depth + 1);
      return node.copy(node.content.replaceChild(index, inner));
    } else if (!slice.content.size) {
      return close(node, replaceTwoWay($from, $to, depth));
    } else if (!slice.openStart && !slice.openEnd && $from.depth == depth && $to.depth == depth) {
      let parent = $from.parent, content = parent.content;
      return close(parent, content.cut(0, $from.parentOffset).append(slice.content).append(content.cut($to.parentOffset)));
    } else {
      let { start, end } = prepareSliceForReplace(slice, $from);
      return close(node, replaceThreeWay($from, start, end, $to, depth));
    }
  }
  function checkJoin(main, sub) {
    if (!sub.type.compatibleContent(main.type))
      throw new ReplaceError("Cannot join " + sub.type.name + " onto " + main.type.name);
  }
  function joinable($before, $after, depth) {
    let node = $before.node(depth);
    checkJoin(node, $after.node(depth));
    return node;
  }
  function addNode(child, target) {
    let last = target.length - 1;
    if (last >= 0 && child.isText && child.sameMarkup(target[last]))
      target[last] = child.withText(target[last].text + child.text);
    else
      target.push(child);
  }
  function addRange($start, $end, depth, target) {
    let node = ($end || $start).node(depth);
    let startIndex = 0, endIndex = $end ? $end.index(depth) : node.childCount;
    if ($start) {
      startIndex = $start.index(depth);
      if ($start.depth > depth) {
        startIndex++;
      } else if ($start.textOffset) {
        addNode($start.nodeAfter, target);
        startIndex++;
      }
    }
    for (let i = startIndex; i < endIndex; i++)
      addNode(node.child(i), target);
    if ($end && $end.depth == depth && $end.textOffset)
      addNode($end.nodeBefore, target);
  }
  function close(node, content) {
    if (!node.type.validContent(content))
      throw new ReplaceError("Invalid content for node " + node.type.name);
    return node.copy(content);
  }
  function replaceThreeWay($from, $start, $end, $to, depth) {
    let openStart = $from.depth > depth && joinable($from, $start, depth + 1);
    let openEnd = $to.depth > depth && joinable($end, $to, depth + 1);
    let content = [];
    addRange(null, $from, depth, content);
    if (openStart && openEnd && $start.index(depth) == $end.index(depth)) {
      checkJoin(openStart, openEnd);
      addNode(close(openStart, replaceThreeWay($from, $start, $end, $to, depth + 1)), content);
    } else {
      if (openStart)
        addNode(close(openStart, replaceTwoWay($from, $start, depth + 1)), content);
      addRange($start, $end, depth, content);
      if (openEnd)
        addNode(close(openEnd, replaceTwoWay($end, $to, depth + 1)), content);
    }
    addRange($to, null, depth, content);
    return new Fragment(content);
  }
  function replaceTwoWay($from, $to, depth) {
    let content = [];
    addRange(null, $from, depth, content);
    if ($from.depth > depth) {
      let type = joinable($from, $to, depth + 1);
      addNode(close(type, replaceTwoWay($from, $to, depth + 1)), content);
    }
    addRange($to, null, depth, content);
    return new Fragment(content);
  }
  function prepareSliceForReplace(slice, $along) {
    let extra = $along.depth - slice.openStart, parent = $along.node(extra);
    let node = parent.copy(slice.content);
    for (let i = extra - 1; i >= 0; i--)
      node = $along.node(i).copy(Fragment.from(node));
    return {
      start: node.resolveNoCache(slice.openStart + extra),
      end: node.resolveNoCache(node.content.size - slice.openEnd - extra)
    };
  }
  var ResolvedPos = class _ResolvedPos {
    /**
    @internal
    */
    constructor(pos, path, parentOffset) {
      this.pos = pos;
      this.path = path;
      this.parentOffset = parentOffset;
      this.depth = path.length / 3 - 1;
    }
    /**
    @internal
    */
    resolveDepth(val) {
      if (val == null)
        return this.depth;
      if (val < 0)
        return this.depth + val;
      return val;
    }
    /**
    The parent node that the position points into. Note that even if
    a position points into a text node, that node is not considered
    the parent—text nodes are ‘flat’ in this model, and have no content.
    */
    get parent() {
      return this.node(this.depth);
    }
    /**
    The root node in which the position was resolved.
    */
    get doc() {
      return this.node(0);
    }
    /**
    The ancestor node at the given level. `p.node(p.depth)` is the
    same as `p.parent`.
    */
    node(depth) {
      return this.path[this.resolveDepth(depth) * 3];
    }
    /**
    The index into the ancestor at the given level. If this points
    at the 3rd node in the 2nd paragraph on the top level, for
    example, `p.index(0)` is 1 and `p.index(1)` is 2.
    */
    index(depth) {
      return this.path[this.resolveDepth(depth) * 3 + 1];
    }
    /**
    The index pointing after this position into the ancestor at the
    given level.
    */
    indexAfter(depth) {
      depth = this.resolveDepth(depth);
      return this.index(depth) + (depth == this.depth && !this.textOffset ? 0 : 1);
    }
    /**
    The (absolute) position at the start of the node at the given
    level.
    */
    start(depth) {
      depth = this.resolveDepth(depth);
      return depth == 0 ? 0 : this.path[depth * 3 - 1] + 1;
    }
    /**
    The (absolute) position at the end of the node at the given
    level.
    */
    end(depth) {
      depth = this.resolveDepth(depth);
      return this.start(depth) + this.node(depth).content.size;
    }
    /**
    The (absolute) position directly before the wrapping node at the
    given level, or, when `depth` is `this.depth + 1`, the original
    position.
    */
    before(depth) {
      depth = this.resolveDepth(depth);
      if (!depth)
        throw new RangeError("There is no position before the top-level node");
      return depth == this.depth + 1 ? this.pos : this.path[depth * 3 - 1];
    }
    /**
    The (absolute) position directly after the wrapping node at the
    given level, or the original position when `depth` is `this.depth + 1`.
    */
    after(depth) {
      depth = this.resolveDepth(depth);
      if (!depth)
        throw new RangeError("There is no position after the top-level node");
      return depth == this.depth + 1 ? this.pos : this.path[depth * 3 - 1] + this.path[depth * 3].nodeSize;
    }
    /**
    When this position points into a text node, this returns the
    distance between the position and the start of the text node.
    Will be zero for positions that point between nodes.
    */
    get textOffset() {
      return this.pos - this.path[this.path.length - 1];
    }
    /**
    Get the node directly after the position, if any. If the position
    points into a text node, only the part of that node after the
    position is returned.
    */
    get nodeAfter() {
      let parent = this.parent, index = this.index(this.depth);
      if (index == parent.childCount)
        return null;
      let dOff = this.pos - this.path[this.path.length - 1], child = parent.child(index);
      return dOff ? parent.child(index).cut(dOff) : child;
    }
    /**
    Get the node directly before the position, if any. If the
    position points into a text node, only the part of that node
    before the position is returned.
    */
    get nodeBefore() {
      let index = this.index(this.depth);
      let dOff = this.pos - this.path[this.path.length - 1];
      if (dOff)
        return this.parent.child(index).cut(0, dOff);
      return index == 0 ? null : this.parent.child(index - 1);
    }
    /**
    Get the position at the given index in the parent node at the
    given depth (which defaults to `this.depth`).
    */
    posAtIndex(index, depth) {
      depth = this.resolveDepth(depth);
      let node = this.path[depth * 3], pos = depth == 0 ? 0 : this.path[depth * 3 - 1] + 1;
      for (let i = 0; i < index; i++)
        pos += node.child(i).nodeSize;
      return pos;
    }
    /**
    Get the marks at this position, factoring in the surrounding
    marks' [`inclusive`](https://prosemirror.net/docs/ref/#model.MarkSpec.inclusive) property. If the
    position is at the start of a non-empty node, the marks of the
    node after it (if any) are returned.
    */
    marks() {
      let parent = this.parent, index = this.index();
      if (parent.content.size == 0)
        return Mark.none;
      if (this.textOffset)
        return parent.child(index).marks;
      let main = parent.maybeChild(index - 1), other = parent.maybeChild(index);
      if (!main) {
        let tmp = main;
        main = other;
        other = tmp;
      }
      let marks = main.marks;
      for (var i = 0; i < marks.length; i++)
        if (marks[i].type.spec.inclusive === false && (!other || !marks[i].isInSet(other.marks)))
          marks = marks[i--].removeFromSet(marks);
      return marks;
    }
    /**
    Get the marks after the current position, if any, except those
    that are non-inclusive and not present at position `$end`. This
    is mostly useful for getting the set of marks to preserve after a
    deletion. Will return `null` if this position is at the end of
    its parent node or its parent node isn't a textblock (in which
    case no marks should be preserved).
    */
    marksAcross($end) {
      let after = this.parent.maybeChild(this.index());
      if (!after || !after.isInline)
        return null;
      let marks = after.marks, next = $end.parent.maybeChild($end.index());
      for (var i = 0; i < marks.length; i++)
        if (marks[i].type.spec.inclusive === false && (!next || !marks[i].isInSet(next.marks)))
          marks = marks[i--].removeFromSet(marks);
      return marks;
    }
    /**
    The depth up to which this position and the given (non-resolved)
    position share the same parent nodes.
    */
    sharedDepth(pos) {
      for (let depth = this.depth; depth > 0; depth--)
        if (this.start(depth) <= pos && this.end(depth) >= pos)
          return depth;
      return 0;
    }
    /**
    Returns a range based on the place where this position and the
    given position diverge around block content. If both point into
    the same textblock, for example, a range around that textblock
    will be returned. If they point into different blocks, the range
    around those blocks in their shared ancestor is returned. You can
    pass in an optional predicate that will be called with a parent
    node to see if a range into that parent is acceptable.
    */
    blockRange(other = this, pred) {
      if (other.pos < this.pos)
        return other.blockRange(this);
      for (let d = this.depth - (this.parent.inlineContent || this.pos == other.pos ? 1 : 0); d >= 0; d--)
        if (other.pos <= this.end(d) && (!pred || pred(this.node(d))))
          return new NodeRange(this, other, d);
      return null;
    }
    /**
    Query whether the given position shares the same parent node.
    */
    sameParent(other) {
      return this.pos - this.parentOffset == other.pos - other.parentOffset;
    }
    /**
    Return the greater of this and the given position.
    */
    max(other) {
      return other.pos > this.pos ? other : this;
    }
    /**
    Return the smaller of this and the given position.
    */
    min(other) {
      return other.pos < this.pos ? other : this;
    }
    /**
    @internal
    */
    toString() {
      let str = "";
      for (let i = 1; i <= this.depth; i++)
        str += (str ? "/" : "") + this.node(i).type.name + "_" + this.index(i - 1);
      return str + ":" + this.parentOffset;
    }
    /**
    @internal
    */
    static resolve(doc2, pos) {
      if (!(pos >= 0 && pos <= doc2.content.size))
        throw new RangeError("Position " + pos + " out of range");
      let path = [];
      let start = 0, parentOffset = pos;
      for (let node = doc2; ; ) {
        let { index, offset } = node.content.findIndex(parentOffset);
        let rem = parentOffset - offset;
        path.push(node, index, start + offset);
        if (!rem)
          break;
        node = node.child(index);
        if (node.isText)
          break;
        parentOffset = rem - 1;
        start += offset + 1;
      }
      return new _ResolvedPos(pos, path, parentOffset);
    }
    /**
    @internal
    */
    static resolveCached(doc2, pos) {
      let cache = resolveCache.get(doc2);
      if (cache) {
        for (let i = 0; i < cache.elts.length; i++) {
          let elt = cache.elts[i];
          if (elt.pos == pos)
            return elt;
        }
      } else {
        resolveCache.set(doc2, cache = new ResolveCache());
      }
      let result = cache.elts[cache.i] = _ResolvedPos.resolve(doc2, pos);
      cache.i = (cache.i + 1) % resolveCacheSize;
      return result;
    }
  };
  var ResolveCache = class {
    constructor() {
      this.elts = [];
      this.i = 0;
    }
  };
  var resolveCacheSize = 12;
  var resolveCache = /* @__PURE__ */ new WeakMap();
  var NodeRange = class {
    /**
    Construct a node range. `$from` and `$to` should point into the
    same node until at least the given `depth`, since a node range
    denotes an adjacent set of nodes in a single parent node.
    */
    constructor($from, $to, depth) {
      this.$from = $from;
      this.$to = $to;
      this.depth = depth;
    }
    /**
    The position at the start of the range.
    */
    get start() {
      return this.$from.before(this.depth + 1);
    }
    /**
    The position at the end of the range.
    */
    get end() {
      return this.$to.after(this.depth + 1);
    }
    /**
    The parent node that the range points into.
    */
    get parent() {
      return this.$from.node(this.depth);
    }
    /**
    The start index of the range in the parent node.
    */
    get startIndex() {
      return this.$from.index(this.depth);
    }
    /**
    The end index of the range in the parent node.
    */
    get endIndex() {
      return this.$to.indexAfter(this.depth);
    }
  };
  var emptyAttrs = /* @__PURE__ */ Object.create(null);
  var Node = class _Node {
    /**
    @internal
    */
    constructor(type, attrs, content, marks = Mark.none) {
      this.type = type;
      this.attrs = attrs;
      this.marks = marks;
      this.content = content || Fragment.empty;
    }
    /**
    The array of this node's child nodes.
    */
    get children() {
      return this.content.content;
    }
    /**
    The size of this node, as defined by the integer-based [indexing
    scheme](https://prosemirror.net/docs/guide/#doc.indexing). For text nodes, this is the
    amount of characters. For other leaf nodes, it is one. For
    non-leaf nodes, it is the size of the content plus two (the
    start and end token).
    */
    get nodeSize() {
      return this.isLeaf ? 1 : 2 + this.content.size;
    }
    /**
    The number of children that the node has.
    */
    get childCount() {
      return this.content.childCount;
    }
    /**
    Get the child node at the given index. Raises an error when the
    index is out of range.
    */
    child(index) {
      return this.content.child(index);
    }
    /**
    Get the child node at the given index, if it exists.
    */
    maybeChild(index) {
      return this.content.maybeChild(index);
    }
    /**
    Call `f` for every child node, passing the node, its offset
    into this parent node, and its index.
    */
    forEach(f) {
      this.content.forEach(f);
    }
    /**
    Invoke a callback for all descendant nodes recursively overlapping
    the given two positions that are relative to start of this
    node's content. This includes all ancestors of the nodes
    containing the two positions. The callback is invoked with the
    node, its position relative to the original node (method receiver),
    its parent node, and its child index. When the callback returns
    false for a given node, that node's children will not be
    recursed over. The last parameter can be used to specify a
    starting position to count from.
    */
    nodesBetween(from, to, f, startPos = 0) {
      this.content.nodesBetween(from, to, f, startPos, this);
    }
    /**
    Call the given callback for every descendant node. Doesn't
    descend into a node when the callback returns `false`.
    */
    descendants(f) {
      this.nodesBetween(0, this.content.size, f);
    }
    /**
    Concatenates all the text nodes found in this fragment and its
    children.
    */
    get textContent() {
      return this.isLeaf && this.type.spec.leafText ? this.type.spec.leafText(this) : this.textBetween(0, this.content.size, "");
    }
    /**
    Get all text between positions `from` and `to`. When
    `blockSeparator` is given, it will be inserted to separate text
    from different block nodes. If `leafText` is given, it'll be
    inserted for every non-text leaf node encountered, otherwise
    [`leafText`](https://prosemirror.net/docs/ref/#model.NodeSpec.leafText) will be used.
    */
    textBetween(from, to, blockSeparator, leafText) {
      return this.content.textBetween(from, to, blockSeparator, leafText);
    }
    /**
    Returns this node's first child, or `null` if there are no
    children.
    */
    get firstChild() {
      return this.content.firstChild;
    }
    /**
    Returns this node's last child, or `null` if there are no
    children.
    */
    get lastChild() {
      return this.content.lastChild;
    }
    /**
    Test whether two nodes represent the same piece of document.
    */
    eq(other) {
      return this == other || this.sameMarkup(other) && this.content.eq(other.content);
    }
    /**
    Compare the markup (type, attributes, and marks) of this node to
    those of another. Returns `true` if both have the same markup.
    */
    sameMarkup(other) {
      return this.hasMarkup(other.type, other.attrs, other.marks);
    }
    /**
    Check whether this node's markup correspond to the given type,
    attributes, and marks.
    */
    hasMarkup(type, attrs, marks) {
      return this.type == type && compareDeep(this.attrs, attrs || type.defaultAttrs || emptyAttrs) && Mark.sameSet(this.marks, marks || Mark.none);
    }
    /**
    Create a new node with the same markup as this node, containing
    the given content (or empty, if no content is given).
    */
    copy(content = null) {
      if (content == this.content)
        return this;
      return new _Node(this.type, this.attrs, content, this.marks);
    }
    /**
    Create a copy of this node, with the given set of marks instead
    of the node's own marks.
    */
    mark(marks) {
      return marks == this.marks ? this : new _Node(this.type, this.attrs, this.content, marks);
    }
    /**
    Create a copy of this node with only the content between the
    given positions. If `to` is not given, it defaults to the end of
    the node.
    */
    cut(from, to = this.content.size) {
      if (from == 0 && to == this.content.size)
        return this;
      return this.copy(this.content.cut(from, to));
    }
    /**
    Cut out the part of the document between the given positions, and
    return it as a `Slice` object.
    */
    slice(from, to = this.content.size, includeParents = false) {
      if (from == to)
        return Slice.empty;
      let $from = this.resolve(from), $to = this.resolve(to);
      let depth = includeParents ? 0 : $from.sharedDepth(to);
      let start = $from.start(depth), node = $from.node(depth);
      let content = node.content.cut($from.pos - start, $to.pos - start);
      return new Slice(content, $from.depth - depth, $to.depth - depth);
    }
    /**
    Replace the part of the document between the given positions with
    the given slice. The slice must 'fit', meaning its open sides
    must be able to connect to the surrounding content, and its
    content nodes must be valid children for the node they are placed
    into. If any of this is violated, an error of type
    [`ReplaceError`](https://prosemirror.net/docs/ref/#model.ReplaceError) is thrown.
    */
    replace(from, to, slice) {
      return replace(this.resolve(from), this.resolve(to), slice);
    }
    /**
    Find the node directly after the given position.
    */
    nodeAt(pos) {
      for (let node = this; ; ) {
        let { index, offset } = node.content.findIndex(pos);
        node = node.maybeChild(index);
        if (!node)
          return null;
        if (offset == pos || node.isText)
          return node;
        pos -= offset + 1;
      }
    }
    /**
    Find the (direct) child node after the given offset, if any,
    and return it along with its index and offset relative to this
    node.
    */
    childAfter(pos) {
      let { index, offset } = this.content.findIndex(pos);
      return { node: this.content.maybeChild(index), index, offset };
    }
    /**
    Find the (direct) child node before the given offset, if any,
    and return it along with its index and offset relative to this
    node.
    */
    childBefore(pos) {
      if (pos == 0)
        return { node: null, index: 0, offset: 0 };
      let { index, offset } = this.content.findIndex(pos);
      if (offset < pos)
        return { node: this.content.child(index), index, offset };
      let node = this.content.child(index - 1);
      return { node, index: index - 1, offset: offset - node.nodeSize };
    }
    /**
    Resolve the given position in the document, returning an
    [object](https://prosemirror.net/docs/ref/#model.ResolvedPos) with information about its context.
    */
    resolve(pos) {
      return ResolvedPos.resolveCached(this, pos);
    }
    /**
    @internal
    */
    resolveNoCache(pos) {
      return ResolvedPos.resolve(this, pos);
    }
    /**
    Test whether a given mark or mark type occurs in this document
    between the two given positions.
    */
    rangeHasMark(from, to, type) {
      let found2 = false;
      if (to > from)
        this.nodesBetween(from, to, (node) => {
          if (type.isInSet(node.marks))
            found2 = true;
          return !found2;
        });
      return found2;
    }
    /**
    True when this is a block (non-inline node)
    */
    get isBlock() {
      return this.type.isBlock;
    }
    /**
    True when this is a textblock node, a block node with inline
    content.
    */
    get isTextblock() {
      return this.type.isTextblock;
    }
    /**
    True when this node allows inline content.
    */
    get inlineContent() {
      return this.type.inlineContent;
    }
    /**
    True when this is an inline node (a text node or a node that can
    appear among text).
    */
    get isInline() {
      return this.type.isInline;
    }
    /**
    True when this is a text node.
    */
    get isText() {
      return this.type.isText;
    }
    /**
    True when this is a leaf node.
    */
    get isLeaf() {
      return this.type.isLeaf;
    }
    /**
    True when this is an atom, i.e. when it does not have directly
    editable content. This is usually the same as `isLeaf`, but can
    be configured with the [`atom` property](https://prosemirror.net/docs/ref/#model.NodeSpec.atom)
    on a node's spec (typically used when the node is displayed as
    an uneditable [node view](https://prosemirror.net/docs/ref/#view.NodeView)).
    */
    get isAtom() {
      return this.type.isAtom;
    }
    /**
    Return a string representation of this node for debugging
    purposes.
    */
    toString() {
      if (this.type.spec.toDebugString)
        return this.type.spec.toDebugString(this);
      let name = this.type.name;
      if (this.content.size)
        name += "(" + this.content.toStringInner() + ")";
      return wrapMarks(this.marks, name);
    }
    /**
    Get the content match in this node at the given index.
    */
    contentMatchAt(index) {
      let match = this.type.contentMatch.matchFragment(this.content, 0, index);
      if (!match)
        throw new Error("Called contentMatchAt on a node with invalid content");
      return match;
    }
    /**
    Test whether replacing the range between `from` and `to` (by
    child index) with the given replacement fragment (which defaults
    to the empty fragment) would leave the node's content valid. You
    can optionally pass `start` and `end` indices into the
    replacement fragment.
    */
    canReplace(from, to, replacement = Fragment.empty, start = 0, end = replacement.childCount) {
      let one = this.contentMatchAt(from).matchFragment(replacement, start, end);
      let two = one && one.matchFragment(this.content, to);
      if (!two || !two.validEnd)
        return false;
      for (let i = start; i < end; i++)
        if (!this.type.allowsMarks(replacement.child(i).marks))
          return false;
      return true;
    }
    /**
    Test whether replacing the range `from` to `to` (by index) with
    a node of the given type would leave the node's content valid.
    */
    canReplaceWith(from, to, type, marks) {
      if (marks && !this.type.allowsMarks(marks))
        return false;
      let start = this.contentMatchAt(from).matchType(type);
      let end = start && start.matchFragment(this.content, to);
      return end ? end.validEnd : false;
    }
    /**
    Test whether the given node's content could be appended to this
    node. If that node is empty, this will only return true if there
    is at least one node type that can appear in both nodes (to avoid
    merging completely incompatible nodes).
    */
    canAppend(other) {
      if (other.content.size)
        return this.canReplace(this.childCount, this.childCount, other.content);
      else
        return this.type.compatibleContent(other.type);
    }
    /**
    Check whether this node and its descendants conform to the
    schema, and raise an exception when they do not.
    */
    check() {
      this.type.checkContent(this.content);
      checkAttrs(this.type.attrs, this.attrs, "node", this.type.name);
      let copy = Mark.none;
      for (let i = 0; i < this.marks.length; i++) {
        let mark = this.marks[i];
        checkAttrs(mark.type.attrs, mark.attrs, "mark", mark.type.name);
        copy = mark.addToSet(copy);
      }
      if (!Mark.sameSet(copy, this.marks))
        throw new RangeError(`Invalid collection of marks for node ${this.type.name}: ${this.marks.map((m) => m.type.name)}`);
      this.content.forEach((node) => node.check());
    }
    /**
    Return a JSON-serializeable representation of this node.
    */
    toJSON() {
      let obj = { type: this.type.name };
      for (let _ in this.attrs) {
        obj.attrs = this.attrs;
        break;
      }
      if (this.content.size)
        obj.content = this.content.toJSON();
      if (this.marks.length)
        obj.marks = this.marks.map((n) => n.toJSON());
      return obj;
    }
    /**
    Deserialize a node from its JSON representation.
    */
    static fromJSON(schema2, json) {
      if (!json)
        throw new RangeError("Invalid input for Node.fromJSON");
      let marks = void 0;
      if (json.marks) {
        if (!Array.isArray(json.marks))
          throw new RangeError("Invalid mark data for Node.fromJSON");
        marks = json.marks.map(schema2.markFromJSON);
      }
      if (json.type == "text") {
        if (typeof json.text != "string")
          throw new RangeError("Invalid text node in JSON");
        return schema2.text(json.text, marks);
      }
      let content = Fragment.fromJSON(schema2, json.content);
      let node = schema2.nodeType(json.type).create(json.attrs, content, marks);
      return node;
    }
  };
  Node.prototype.text = void 0;
  var TextNode = class _TextNode extends Node {
    /**
    @internal
    */
    constructor(type, attrs, content, marks) {
      super(type, attrs, null, marks);
      if (!content)
        throw new RangeError("Empty text nodes are not allowed");
      this.text = content;
    }
    toString() {
      if (this.type.spec.toDebugString)
        return this.type.spec.toDebugString(this);
      return wrapMarks(this.marks, JSON.stringify(this.text));
    }
    get textContent() {
      return this.text;
    }
    textBetween(from, to) {
      return this.text.slice(from, to);
    }
    get nodeSize() {
      return this.text.length;
    }
    mark(marks) {
      return marks == this.marks ? this : new _TextNode(this.type, this.attrs, this.text, marks);
    }
    withText(text) {
      if (text == this.text)
        return this;
      return new _TextNode(this.type, this.attrs, text, this.marks);
    }
    cut(from = 0, to = this.text.length) {
      if (from == 0 && to == this.text.length)
        return this;
      return this.withText(this.text.slice(from, to));
    }
    eq(other) {
      return this.sameMarkup(other) && this.text == other.text;
    }
    toJSON() {
      let base = super.toJSON();
      base.text = this.text;
      return base;
    }
  };
  function wrapMarks(marks, str) {
    for (let i = marks.length - 1; i >= 0; i--)
      str = marks[i].type.name + "(" + str + ")";
    return str;
  }
  function checkAttrs(attrs, values, type, name) {
    for (let attr in values)
      if (!(attr in attrs))
        throw new RangeError(`Unsupported attribute ${attr} for ${type} of type ${name}`);
    for (let attr in attrs) {
      if (attrs[attr].validate)
        attrs[attr].validate(values[attr]);
    }
  }
  var ContentMatch = class _ContentMatch {
    /**
    @internal
    */
    constructor(validEnd) {
      this.validEnd = validEnd;
      this.next = [];
      this.wrapCache = [];
    }
    /**
    @internal
    */
    static parse(string, nodeTypes) {
      let stream = new TokenStream(string, nodeTypes);
      if (stream.next == null)
        return _ContentMatch.empty;
      let expr = parseExpr(stream);
      if (stream.next)
        stream.err("Unexpected trailing text");
      let match = dfa(nfa(expr));
      checkForDeadEnds(match, stream);
      return match;
    }
    /**
    Match a node type, returning a match after that node if
    successful.
    */
    matchType(type) {
      for (let i = 0; i < this.next.length; i++)
        if (this.next[i].type == type)
          return this.next[i].next;
      return null;
    }
    /**
    Try to match a fragment. Returns the resulting match when
    successful.
    */
    matchFragment(frag, start = 0, end = frag.childCount) {
      let cur = this;
      for (let i = start; cur && i < end; i++)
        cur = cur.matchType(frag.child(i).type);
      return cur;
    }
    /**
    @internal
    */
    get inlineContent() {
      return this.next.length != 0 && this.next[0].type.isInline;
    }
    /**
    Get the first matching node type at this match position that can
    be generated.
    */
    get defaultType() {
      for (let i = 0; i < this.next.length; i++) {
        let { type } = this.next[i];
        if (!(type.isText || type.hasRequiredAttrs()))
          return type;
      }
      return null;
    }
    /**
    @internal
    */
    compatible(other) {
      for (let i = 0; i < this.next.length; i++)
        for (let j = 0; j < other.next.length; j++)
          if (this.next[i].type == other.next[j].type)
            return true;
      return false;
    }
    /**
    Try to match the given fragment, and if that fails, see if it can
    be made to match by inserting nodes in front of it. When
    successful, return a fragment of inserted nodes (which may be
    empty if nothing had to be inserted). When `toEnd` is true, only
    return a fragment if the resulting match goes to the end of the
    content expression.
    */
    fillBefore(after, toEnd = false, startIndex = 0) {
      let seen = [this];
      function search(match, types) {
        let finished = match.matchFragment(after, startIndex);
        if (finished && (!toEnd || finished.validEnd))
          return Fragment.from(types.map((tp) => tp.createAndFill()));
        for (let i = 0; i < match.next.length; i++) {
          let { type, next } = match.next[i];
          if (!(type.isText || type.hasRequiredAttrs()) && seen.indexOf(next) == -1) {
            seen.push(next);
            let found2 = search(next, types.concat(type));
            if (found2)
              return found2;
          }
        }
        return null;
      }
      return search(this, []);
    }
    /**
    Find a set of wrapping node types that would allow a node of the
    given type to appear at this position. The result may be empty
    (when it fits directly) and will be null when no such wrapping
    exists.
    */
    findWrapping(target) {
      for (let i = 0; i < this.wrapCache.length; i += 2)
        if (this.wrapCache[i] == target)
          return this.wrapCache[i + 1];
      let computed = this.computeWrapping(target);
      this.wrapCache.push(target, computed);
      return computed;
    }
    /**
    @internal
    */
    computeWrapping(target) {
      let seen = /* @__PURE__ */ Object.create(null), active = [{ match: this, type: null, via: null }];
      while (active.length) {
        let current = active.shift(), match = current.match;
        if (match.matchType(target)) {
          let result = [];
          for (let obj = current; obj.type; obj = obj.via)
            result.push(obj.type);
          return result.reverse();
        }
        for (let i = 0; i < match.next.length; i++) {
          let { type, next } = match.next[i];
          if (!type.isLeaf && !type.hasRequiredAttrs() && !(type.name in seen) && (!current.type || next.validEnd)) {
            active.push({ match: type.contentMatch, type, via: current });
            seen[type.name] = true;
          }
        }
      }
      return null;
    }
    /**
    The number of outgoing edges this node has in the finite
    automaton that describes the content expression.
    */
    get edgeCount() {
      return this.next.length;
    }
    /**
    Get the _n_​th outgoing edge from this node in the finite
    automaton that describes the content expression.
    */
    edge(n) {
      if (n >= this.next.length)
        throw new RangeError(`There's no ${n}th edge in this content match`);
      return this.next[n];
    }
    /**
    @internal
    */
    toString() {
      let seen = [];
      function scan(m) {
        seen.push(m);
        for (let i = 0; i < m.next.length; i++)
          if (seen.indexOf(m.next[i].next) == -1)
            scan(m.next[i].next);
      }
      scan(this);
      return seen.map((m, i) => {
        let out = i + (m.validEnd ? "*" : " ") + " ";
        for (let i2 = 0; i2 < m.next.length; i2++)
          out += (i2 ? ", " : "") + m.next[i2].type.name + "->" + seen.indexOf(m.next[i2].next);
        return out;
      }).join("\n");
    }
  };
  ContentMatch.empty = new ContentMatch(true);
  var TokenStream = class {
    constructor(string, nodeTypes) {
      this.string = string;
      this.nodeTypes = nodeTypes;
      this.inline = null;
      this.pos = 0;
      this.tokens = string.split(/\s*(?=\b|\W|$)/);
      if (this.tokens[this.tokens.length - 1] == "")
        this.tokens.pop();
      if (this.tokens[0] == "")
        this.tokens.shift();
    }
    get next() {
      return this.tokens[this.pos];
    }
    eat(tok) {
      return this.next == tok && (this.pos++ || true);
    }
    err(str) {
      throw new SyntaxError(str + " (in content expression '" + this.string + "')");
    }
  };
  function parseExpr(stream) {
    let exprs = [];
    do {
      exprs.push(parseExprSeq(stream));
    } while (stream.eat("|"));
    return exprs.length == 1 ? exprs[0] : { type: "choice", exprs };
  }
  function parseExprSeq(stream) {
    let exprs = [];
    do {
      exprs.push(parseExprSubscript(stream));
    } while (stream.next && stream.next != ")" && stream.next != "|");
    return exprs.length == 1 ? exprs[0] : { type: "seq", exprs };
  }
  function parseExprSubscript(stream) {
    let expr = parseExprAtom(stream);
    for (; ; ) {
      if (stream.eat("+"))
        expr = { type: "plus", expr };
      else if (stream.eat("*"))
        expr = { type: "star", expr };
      else if (stream.eat("?"))
        expr = { type: "opt", expr };
      else if (stream.eat("{"))
        expr = parseExprRange(stream, expr);
      else
        break;
    }
    return expr;
  }
  function parseNum(stream) {
    if (/\D/.test(stream.next))
      stream.err("Expected number, got '" + stream.next + "'");
    let result = Number(stream.next);
    stream.pos++;
    return result;
  }
  function parseExprRange(stream, expr) {
    let min = parseNum(stream), max = min;
    if (stream.eat(",")) {
      if (stream.next != "}")
        max = parseNum(stream);
      else
        max = -1;
    }
    if (!stream.eat("}"))
      stream.err("Unclosed braced range");
    return { type: "range", min, max, expr };
  }
  function resolveName(stream, name) {
    let types = stream.nodeTypes, type = types[name];
    if (type)
      return [type];
    let result = [];
    for (let typeName in types) {
      let type2 = types[typeName];
      if (type2.isInGroup(name))
        result.push(type2);
    }
    if (result.length == 0)
      stream.err("No node type or group '" + name + "' found");
    return result;
  }
  function parseExprAtom(stream) {
    if (stream.eat("(")) {
      let expr = parseExpr(stream);
      if (!stream.eat(")"))
        stream.err("Missing closing paren");
      return expr;
    } else if (!/\W/.test(stream.next)) {
      let exprs = resolveName(stream, stream.next).map((type) => {
        if (stream.inline == null)
          stream.inline = type.isInline;
        else if (stream.inline != type.isInline)
          stream.err("Mixing inline and block content");
        return { type: "name", value: type };
      });
      stream.pos++;
      return exprs.length == 1 ? exprs[0] : { type: "choice", exprs };
    } else {
      stream.err("Unexpected token '" + stream.next + "'");
    }
  }
  function nfa(expr) {
    let nfa2 = [[]];
    connect(compile(expr, 0), node());
    return nfa2;
    function node() {
      return nfa2.push([]) - 1;
    }
    function edge(from, to, term) {
      let edge2 = { term, to };
      nfa2[from].push(edge2);
      return edge2;
    }
    function connect(edges, to) {
      edges.forEach((edge2) => edge2.to = to);
    }
    function compile(expr2, from) {
      if (expr2.type == "choice") {
        return expr2.exprs.reduce((out, expr3) => out.concat(compile(expr3, from)), []);
      } else if (expr2.type == "seq") {
        for (let i = 0; ; i++) {
          let next = compile(expr2.exprs[i], from);
          if (i == expr2.exprs.length - 1)
            return next;
          connect(next, from = node());
        }
      } else if (expr2.type == "star") {
        let loop = node();
        edge(from, loop);
        connect(compile(expr2.expr, loop), loop);
        return [edge(loop)];
      } else if (expr2.type == "plus") {
        let loop = node();
        connect(compile(expr2.expr, from), loop);
        connect(compile(expr2.expr, loop), loop);
        return [edge(loop)];
      } else if (expr2.type == "opt") {
        return [edge(from)].concat(compile(expr2.expr, from));
      } else if (expr2.type == "range") {
        let cur = from;
        for (let i = 0; i < expr2.min; i++) {
          let next = node();
          connect(compile(expr2.expr, cur), next);
          cur = next;
        }
        if (expr2.max == -1) {
          connect(compile(expr2.expr, cur), cur);
        } else {
          for (let i = expr2.min; i < expr2.max; i++) {
            let next = node();
            edge(cur, next);
            connect(compile(expr2.expr, cur), next);
            cur = next;
          }
        }
        return [edge(cur)];
      } else if (expr2.type == "name") {
        return [edge(from, void 0, expr2.value)];
      } else {
        throw new Error("Unknown expr type");
      }
    }
  }
  function cmp(a, b) {
    return b - a;
  }
  function nullFrom(nfa2, node) {
    let result = [];
    scan(node);
    return result.sort(cmp);
    function scan(node2) {
      let edges = nfa2[node2];
      if (edges.length == 1 && !edges[0].term)
        return scan(edges[0].to);
      result.push(node2);
      for (let i = 0; i < edges.length; i++) {
        let { term, to } = edges[i];
        if (!term && result.indexOf(to) == -1)
          scan(to);
      }
    }
  }
  function dfa(nfa2) {
    let labeled = /* @__PURE__ */ Object.create(null);
    return explore(nullFrom(nfa2, 0));
    function explore(states) {
      let out = [];
      states.forEach((node) => {
        nfa2[node].forEach(({ term, to }) => {
          if (!term)
            return;
          let set;
          for (let i = 0; i < out.length; i++)
            if (out[i][0] == term)
              set = out[i][1];
          nullFrom(nfa2, to).forEach((node2) => {
            if (!set)
              out.push([term, set = []]);
            if (set.indexOf(node2) == -1)
              set.push(node2);
          });
        });
      });
      let state = labeled[states.join(",")] = new ContentMatch(states.indexOf(nfa2.length - 1) > -1);
      for (let i = 0; i < out.length; i++) {
        let states2 = out[i][1].sort(cmp);
        state.next.push({ type: out[i][0], next: labeled[states2.join(",")] || explore(states2) });
      }
      return state;
    }
  }
  function checkForDeadEnds(match, stream) {
    for (let i = 0, work = [match]; i < work.length; i++) {
      let state = work[i], dead = !state.validEnd, nodes = [];
      for (let j = 0; j < state.next.length; j++) {
        let { type, next } = state.next[j];
        nodes.push(type.name);
        if (dead && !(type.isText || type.hasRequiredAttrs()))
          dead = false;
        if (work.indexOf(next) == -1)
          work.push(next);
      }
      if (dead)
        stream.err("Only non-generatable nodes (" + nodes.join(", ") + ") in a required position (see https://prosemirror.net/docs/guide/#generatable)");
    }
  }
  function defaultAttrs(attrs) {
    let defaults = /* @__PURE__ */ Object.create(null);
    for (let attrName in attrs) {
      let attr = attrs[attrName];
      if (!attr.hasDefault)
        return null;
      defaults[attrName] = attr.default;
    }
    return defaults;
  }
  function computeAttrs(attrs, value) {
    let built = /* @__PURE__ */ Object.create(null);
    for (let name in attrs) {
      let attr = attrs[name];
      let given = value && value[name];
      if (given === void 0) {
        if (attr.hasDefault)
          given = attr.default;
        else
          throw new RangeError("No value supplied for attribute " + name);
      } else if (attr.validate) {
        attr.validate(given);
      }
      built[name] = given;
    }
    return built;
  }
  function initAttrs(typeName, attrs) {
    let result = /* @__PURE__ */ Object.create(null);
    if (attrs)
      for (let name in attrs)
        result[name] = new Attribute(typeName, name, attrs[name]);
    return result;
  }
  var NodeType = class _NodeType {
    /**
    @internal
    */
    constructor(name, schema2, spec) {
      this.name = name;
      this.schema = schema2;
      this.spec = spec;
      this.markSet = null;
      this.groups = spec.group ? spec.group.split(" ") : [];
      this.attrs = initAttrs(name, spec.attrs);
      this.defaultAttrs = defaultAttrs(this.attrs);
      this.contentMatch = null;
      this.inlineContent = null;
      this.isBlock = !(spec.inline || name == "text");
      this.isText = name == "text";
    }
    /**
    True if this is an inline type.
    */
    get isInline() {
      return !this.isBlock;
    }
    /**
    True if this is a textblock type, a block that contains inline
    content.
    */
    get isTextblock() {
      return this.isBlock && this.inlineContent;
    }
    /**
    True for node types that allow no content.
    */
    get isLeaf() {
      return this.contentMatch == ContentMatch.empty;
    }
    /**
    True when this node is an atom, i.e. when it does not have
    directly editable content.
    */
    get isAtom() {
      return this.isLeaf || !!this.spec.atom;
    }
    /**
    Return true when this node type is part of the given
    [group](https://prosemirror.net/docs/ref/#model.NodeSpec.group).
    */
    isInGroup(group) {
      return this.groups.indexOf(group) > -1;
    }
    /**
    The node type's [whitespace](https://prosemirror.net/docs/ref/#model.NodeSpec.whitespace) option.
    */
    get whitespace() {
      return this.spec.whitespace || (this.spec.code ? "pre" : "normal");
    }
    /**
    Tells you whether this node type has any required attributes.
    */
    hasRequiredAttrs() {
      for (let n in this.attrs)
        if (this.attrs[n].isRequired)
          return true;
      return false;
    }
    /**
    Indicates whether this node allows some of the same content as
    the given node type.
    */
    compatibleContent(other) {
      return this == other || this.contentMatch.compatible(other.contentMatch);
    }
    /**
    @internal
    */
    computeAttrs(attrs) {
      if (!attrs && this.defaultAttrs)
        return this.defaultAttrs;
      else
        return computeAttrs(this.attrs, attrs);
    }
    /**
    Create a `Node` of this type. The given attributes are
    checked and defaulted (you can pass `null` to use the type's
    defaults entirely, if no required attributes exist). `content`
    may be a `Fragment`, a node, an array of nodes, or
    `null`. Similarly `marks` may be `null` to default to the empty
    set of marks.
    */
    create(attrs = null, content, marks) {
      if (this.isText)
        throw new Error("NodeType.create can't construct text nodes");
      return new Node(this, this.computeAttrs(attrs), Fragment.from(content), Mark.setFrom(marks));
    }
    /**
    Like [`create`](https://prosemirror.net/docs/ref/#model.NodeType.create), but check the given content
    against the node type's content restrictions, and throw an error
    if it doesn't match.
    */
    createChecked(attrs = null, content, marks) {
      content = Fragment.from(content);
      this.checkContent(content);
      return new Node(this, this.computeAttrs(attrs), content, Mark.setFrom(marks));
    }
    /**
    Like [`create`](https://prosemirror.net/docs/ref/#model.NodeType.create), but see if it is
    necessary to add nodes to the start or end of the given fragment
    to make it fit the node. If no fitting wrapping can be found,
    return null. Note that, due to the fact that required nodes can
    always be created, this will always succeed if you pass null or
    `Fragment.empty` as content.
    */
    createAndFill(attrs = null, content, marks) {
      attrs = this.computeAttrs(attrs);
      content = Fragment.from(content);
      if (content.size) {
        let before = this.contentMatch.fillBefore(content);
        if (!before)
          return null;
        content = before.append(content);
      }
      let matched = this.contentMatch.matchFragment(content);
      let after = matched && matched.fillBefore(Fragment.empty, true);
      if (!after)
        return null;
      return new Node(this, attrs, content.append(after), Mark.setFrom(marks));
    }
    /**
    Returns true if the given fragment is valid content for this node
    type.
    */
    validContent(content) {
      let result = this.contentMatch.matchFragment(content);
      if (!result || !result.validEnd)
        return false;
      for (let i = 0; i < content.childCount; i++)
        if (!this.allowsMarks(content.child(i).marks))
          return false;
      return true;
    }
    /**
    Throws a RangeError if the given fragment is not valid content for this
    node type.
    @internal
    */
    checkContent(content) {
      if (!this.validContent(content))
        throw new RangeError(`Invalid content for node ${this.name}: ${content.toString().slice(0, 50)}`);
    }
    /**
    @internal no longer useful, but called by old prosemirror-view versions
    */
    checkAttrs(attrs) {
    }
    /**
    Check whether the given mark type is allowed in this node.
    */
    allowsMarkType(markType) {
      return this.markSet == null || this.markSet.indexOf(markType) > -1;
    }
    /**
    Test whether the given set of marks are allowed in this node.
    */
    allowsMarks(marks) {
      if (this.markSet == null)
        return true;
      for (let i = 0; i < marks.length; i++)
        if (!this.allowsMarkType(marks[i].type))
          return false;
      return true;
    }
    /**
    Removes the marks that are not allowed in this node from the given set.
    */
    allowedMarks(marks) {
      if (this.markSet == null)
        return marks;
      let copy;
      for (let i = 0; i < marks.length; i++) {
        if (!this.allowsMarkType(marks[i].type)) {
          if (!copy)
            copy = marks.slice(0, i);
        } else if (copy) {
          copy.push(marks[i]);
        }
      }
      return !copy ? marks : copy.length ? copy : Mark.none;
    }
    /**
    @internal
    */
    static compile(nodes, schema2) {
      let result = /* @__PURE__ */ Object.create(null);
      nodes.forEach((name, spec) => result[name] = new _NodeType(name, schema2, spec));
      let topType = schema2.spec.topNode || "doc";
      if (!result[topType])
        throw new RangeError("Schema is missing its top node type ('" + topType + "')");
      if (!result.text)
        throw new RangeError("Every schema needs a 'text' type");
      for (let _ in result.text.attrs)
        throw new RangeError("The text node type should not have attributes");
      return result;
    }
  };
  function validateType(typeName, attrName, type) {
    let types = type.split("|");
    return (value) => {
      let name = value === null ? "null" : typeof value;
      if (types.indexOf(name) < 0)
        throw new RangeError(`Expected value of type ${types} for attribute ${attrName} on type ${typeName}, got ${name}`);
    };
  }
  var Attribute = class {
    constructor(typeName, attrName, options) {
      this.hasDefault = Object.prototype.hasOwnProperty.call(options, "default");
      this.default = options.default;
      this.validate = typeof options.validate == "string" ? validateType(typeName, attrName, options.validate) : options.validate;
    }
    get isRequired() {
      return !this.hasDefault;
    }
  };
  var MarkType = class _MarkType {
    /**
    @internal
    */
    constructor(name, rank, schema2, spec) {
      this.name = name;
      this.rank = rank;
      this.schema = schema2;
      this.spec = spec;
      this.attrs = initAttrs(name, spec.attrs);
      this.excluded = null;
      let defaults = defaultAttrs(this.attrs);
      this.instance = defaults ? new Mark(this, defaults) : null;
    }
    /**
    Create a mark of this type. `attrs` may be `null` or an object
    containing only some of the mark's attributes. The others, if
    they have defaults, will be added.
    */
    create(attrs = null) {
      if (!attrs && this.instance)
        return this.instance;
      return new Mark(this, computeAttrs(this.attrs, attrs));
    }
    /**
    @internal
    */
    static compile(marks, schema2) {
      let result = /* @__PURE__ */ Object.create(null), rank = 0;
      marks.forEach((name, spec) => result[name] = new _MarkType(name, rank++, schema2, spec));
      return result;
    }
    /**
    When there is a mark of this type in the given set, a new set
    without it is returned. Otherwise, the input set is returned.
    */
    removeFromSet(set) {
      for (var i = 0; i < set.length; i++)
        if (set[i].type == this) {
          set = set.slice(0, i).concat(set.slice(i + 1));
          i--;
        }
      return set;
    }
    /**
    Tests whether there is a mark of this type in the given set.
    */
    isInSet(set) {
      for (let i = 0; i < set.length; i++)
        if (set[i].type == this)
          return set[i];
    }
    /**
    Queries whether a given mark type is
    [excluded](https://prosemirror.net/docs/ref/#model.MarkSpec.excludes) by this one.
    */
    excludes(other) {
      return this.excluded.indexOf(other) > -1;
    }
  };
  var Schema = class {
    /**
    Construct a schema from a schema [specification](https://prosemirror.net/docs/ref/#model.SchemaSpec).
    */
    constructor(spec) {
      this.linebreakReplacement = null;
      this.cached = /* @__PURE__ */ Object.create(null);
      let instanceSpec = this.spec = {};
      for (let prop in spec)
        instanceSpec[prop] = spec[prop];
      instanceSpec.nodes = dist_default.from(spec.nodes), instanceSpec.marks = dist_default.from(spec.marks || {}), this.nodes = NodeType.compile(this.spec.nodes, this);
      this.marks = MarkType.compile(this.spec.marks, this);
      let contentExprCache = /* @__PURE__ */ Object.create(null);
      for (let prop in this.nodes) {
        if (prop in this.marks)
          throw new RangeError(prop + " can not be both a node and a mark");
        let type = this.nodes[prop], contentExpr = type.spec.content || "", markExpr = type.spec.marks;
        type.contentMatch = contentExprCache[contentExpr] || (contentExprCache[contentExpr] = ContentMatch.parse(contentExpr, this.nodes));
        type.inlineContent = type.contentMatch.inlineContent;
        if (type.spec.linebreakReplacement) {
          if (this.linebreakReplacement)
            throw new RangeError("Multiple linebreak nodes defined");
          if (!type.isInline || !type.isLeaf)
            throw new RangeError("Linebreak replacement nodes must be inline leaf nodes");
          this.linebreakReplacement = type;
        }
        type.markSet = markExpr == "_" ? null : markExpr ? gatherMarks(this, markExpr.split(" ")) : markExpr == "" || !type.inlineContent ? [] : null;
      }
      for (let prop in this.marks) {
        let type = this.marks[prop], excl = type.spec.excludes;
        type.excluded = excl == null ? [type] : excl == "" ? [] : gatherMarks(this, excl.split(" "));
      }
      this.nodeFromJSON = (json) => Node.fromJSON(this, json);
      this.markFromJSON = (json) => Mark.fromJSON(this, json);
      this.topNodeType = this.nodes[this.spec.topNode || "doc"];
      this.cached.wrappings = /* @__PURE__ */ Object.create(null);
    }
    /**
    Create a node in this schema. The `type` may be a string or a
    `NodeType` instance. Attributes will be extended with defaults,
    `content` may be a `Fragment`, `null`, a `Node`, or an array of
    nodes.
    */
    node(type, attrs = null, content, marks) {
      if (typeof type == "string")
        type = this.nodeType(type);
      else if (!(type instanceof NodeType))
        throw new RangeError("Invalid node type: " + type);
      else if (type.schema != this)
        throw new RangeError("Node type from different schema used (" + type.name + ")");
      return type.createChecked(attrs, content, marks);
    }
    /**
    Create a text node in the schema. Empty text nodes are not
    allowed.
    */
    text(text, marks) {
      let type = this.nodes.text;
      return new TextNode(type, type.defaultAttrs, text, Mark.setFrom(marks));
    }
    /**
    Create a mark with the given type and attributes.
    */
    mark(type, attrs) {
      if (typeof type == "string")
        type = this.marks[type];
      return type.create(attrs);
    }
    /**
    @internal
    */
    nodeType(name) {
      let found2 = this.nodes[name];
      if (!found2)
        throw new RangeError("Unknown node type: " + name);
      return found2;
    }
  };
  function gatherMarks(schema2, marks) {
    let found2 = [];
    for (let i = 0; i < marks.length; i++) {
      let name = marks[i], mark = schema2.marks[name], ok = mark;
      if (mark) {
        found2.push(mark);
      } else {
        for (let prop in schema2.marks) {
          let mark2 = schema2.marks[prop];
          if (name == "_" || mark2.spec.group && mark2.spec.group.split(" ").indexOf(name) > -1)
            found2.push(ok = mark2);
        }
      }
      if (!ok)
        throw new SyntaxError("Unknown mark type: '" + marks[i] + "'");
    }
    return found2;
  }

  // node_modules/prosemirror-transform/dist/index.js
  var lower16 = 65535;
  var factor16 = Math.pow(2, 16);
  function makeRecover(index, offset) {
    return index + offset * factor16;
  }
  function recoverIndex(value) {
    return value & lower16;
  }
  function recoverOffset(value) {
    return (value - (value & lower16)) / factor16;
  }
  var DEL_BEFORE = 1;
  var DEL_AFTER = 2;
  var DEL_ACROSS = 4;
  var DEL_SIDE = 8;
  var MapResult = class {
    /**
    @internal
    */
    constructor(pos, delInfo, recover) {
      this.pos = pos;
      this.delInfo = delInfo;
      this.recover = recover;
    }
    /**
    Tells you whether the position was deleted, that is, whether the
    step removed the token on the side queried (via the `assoc`)
    argument from the document.
    */
    get deleted() {
      return (this.delInfo & DEL_SIDE) > 0;
    }
    /**
    Tells you whether the token before the mapped position was deleted.
    */
    get deletedBefore() {
      return (this.delInfo & (DEL_BEFORE | DEL_ACROSS)) > 0;
    }
    /**
    True when the token after the mapped position was deleted.
    */
    get deletedAfter() {
      return (this.delInfo & (DEL_AFTER | DEL_ACROSS)) > 0;
    }
    /**
    Tells whether any of the steps mapped through deletes across the
    position (including both the token before and after the
    position).
    */
    get deletedAcross() {
      return (this.delInfo & DEL_ACROSS) > 0;
    }
  };
  var StepMap = class _StepMap {
    /**
    Create a position map. The modifications to the document are
    represented as an array of numbers, in which each group of three
    represents a modified chunk as `[start, oldSize, newSize]`.
    */
    constructor(ranges, inverted = false) {
      this.ranges = ranges;
      this.inverted = inverted;
      if (!ranges.length && _StepMap.empty)
        return _StepMap.empty;
    }
    /**
    @internal
    */
    recover(value) {
      let diff = 0, index = recoverIndex(value);
      if (!this.inverted)
        for (let i = 0; i < index; i++)
          diff += this.ranges[i * 3 + 2] - this.ranges[i * 3 + 1];
      return this.ranges[index * 3] + diff + recoverOffset(value);
    }
    mapResult(pos, assoc = 1) {
      return this._map(pos, assoc, false);
    }
    map(pos, assoc = 1) {
      return this._map(pos, assoc, true);
    }
    /**
    @internal
    */
    _map(pos, assoc, simple) {
      let diff = 0, oldIndex = this.inverted ? 2 : 1, newIndex = this.inverted ? 1 : 2;
      for (let i = 0; i < this.ranges.length; i += 3) {
        let start = this.ranges[i] - (this.inverted ? diff : 0);
        if (start > pos)
          break;
        let oldSize = this.ranges[i + oldIndex], newSize = this.ranges[i + newIndex], end = start + oldSize;
        if (pos <= end) {
          let side = !oldSize ? assoc : pos == start ? -1 : pos == end ? 1 : assoc;
          let result = start + diff + (side < 0 ? 0 : newSize);
          if (simple)
            return result;
          let recover = pos == (assoc < 0 ? start : end) ? null : makeRecover(i / 3, pos - start);
          let del = 0;
          if (oldSize) {
            del |= pos == start ? DEL_AFTER : pos == end ? DEL_BEFORE : DEL_ACROSS;
            if (assoc < 0 ? pos != start : pos != end)
              del |= DEL_SIDE;
          }
          return new MapResult(result, del, recover);
        }
        diff += newSize - oldSize;
      }
      return simple ? pos + diff : new MapResult(pos + diff, 0, null);
    }
    /**
    @internal
    */
    touches(pos, recover) {
      let diff = 0, index = recoverIndex(recover);
      let oldIndex = this.inverted ? 2 : 1, newIndex = this.inverted ? 1 : 2;
      for (let i = 0; i < this.ranges.length; i += 3) {
        let start = this.ranges[i] - (this.inverted ? diff : 0);
        if (start > pos)
          break;
        let oldSize = this.ranges[i + oldIndex], end = start + oldSize;
        if (pos <= end && i == index * 3)
          return true;
        diff += this.ranges[i + newIndex] - oldSize;
      }
      return false;
    }
    /**
    Calls the given function on each of the changed ranges included in
    this map.
    */
    forEach(f) {
      let oldIndex = this.inverted ? 2 : 1, newIndex = this.inverted ? 1 : 2;
      for (let i = 0, diff = 0; i < this.ranges.length; i += 3) {
        let start = this.ranges[i], oldStart = start - (this.inverted ? diff : 0), newStart = start + (this.inverted ? 0 : diff);
        let oldSize = this.ranges[i + oldIndex], newSize = this.ranges[i + newIndex];
        f(oldStart, oldStart + oldSize, newStart, newStart + newSize);
        diff += newSize - oldSize;
      }
    }
    /**
    Create an inverted version of this map. The result can be used to
    map positions in the post-step document to the pre-step document.
    */
    invert() {
      return new _StepMap(this.ranges, !this.inverted);
    }
    /**
    @internal
    */
    toString() {
      return (this.inverted ? "-" : "") + JSON.stringify(this.ranges);
    }
    /**
    Create a map that moves all positions by offset `n` (which may be
    negative). This can be useful when applying steps meant for a
    sub-document to a larger document, or vice-versa.
    */
    static offset(n) {
      return n == 0 ? _StepMap.empty : new _StepMap(n < 0 ? [0, -n, 0] : [0, 0, n]);
    }
  };
  StepMap.empty = new StepMap([]);
  var Mapping = class _Mapping {
    /**
    Create a new mapping with the given position maps.
    */
    constructor(maps, mirror, from = 0, to = maps ? maps.length : 0) {
      this.mirror = mirror;
      this.from = from;
      this.to = to;
      this._maps = maps || [];
      this.ownData = !(maps || mirror);
    }
    /**
    The step maps in this mapping.
    */
    get maps() {
      return this._maps;
    }
    /**
    Create a mapping that maps only through a part of this one.
    */
    slice(from = 0, to = this.maps.length) {
      return new _Mapping(this._maps, this.mirror, from, to);
    }
    /**
    Add a step map to the end of this mapping. If `mirrors` is
    given, it should be the index of the step map that is the mirror
    image of this one.
    */
    appendMap(map, mirrors) {
      if (!this.ownData) {
        this._maps = this._maps.slice();
        this.mirror = this.mirror && this.mirror.slice();
        this.ownData = true;
      }
      this.to = this._maps.push(map);
      if (mirrors != null)
        this.setMirror(this._maps.length - 1, mirrors);
    }
    /**
    Add all the step maps in a given mapping to this one (preserving
    mirroring information).
    */
    appendMapping(mapping) {
      for (let i = 0, startSize = this._maps.length; i < mapping._maps.length; i++) {
        let mirr = mapping.getMirror(i);
        this.appendMap(mapping._maps[i], mirr != null && mirr < i ? startSize + mirr : void 0);
      }
    }
    /**
    Finds the offset of the step map that mirrors the map at the
    given offset, in this mapping (as per the second argument to
    `appendMap`).
    */
    getMirror(n) {
      if (this.mirror) {
        for (let i = 0; i < this.mirror.length; i++)
          if (this.mirror[i] == n)
            return this.mirror[i + (i % 2 ? -1 : 1)];
      }
    }
    /**
    @internal
    */
    setMirror(n, m) {
      if (!this.mirror)
        this.mirror = [];
      this.mirror.push(n, m);
    }
    /**
    Append the inverse of the given mapping to this one.
    */
    appendMappingInverted(mapping) {
      for (let i = mapping.maps.length - 1, totalSize = this._maps.length + mapping._maps.length; i >= 0; i--) {
        let mirr = mapping.getMirror(i);
        this.appendMap(mapping._maps[i].invert(), mirr != null && mirr > i ? totalSize - mirr - 1 : void 0);
      }
    }
    /**
    Create an inverted version of this mapping.
    */
    invert() {
      let inverse = new _Mapping();
      inverse.appendMappingInverted(this);
      return inverse;
    }
    /**
    Map a position through this mapping.
    */
    map(pos, assoc = 1) {
      if (this.mirror)
        return this._map(pos, assoc, true);
      for (let i = this.from; i < this.to; i++)
        pos = this._maps[i].map(pos, assoc);
      return pos;
    }
    /**
    Map a position through this mapping, returning a mapping
    result.
    */
    mapResult(pos, assoc = 1) {
      return this._map(pos, assoc, false);
    }
    /**
    @internal
    */
    _map(pos, assoc, simple) {
      let delInfo = 0;
      for (let i = this.from; i < this.to; i++) {
        let map = this._maps[i], result = map.mapResult(pos, assoc);
        if (result.recover != null) {
          let corr = this.getMirror(i);
          if (corr != null && corr > i && corr < this.to) {
            i = corr;
            pos = this._maps[corr].recover(result.recover);
            continue;
          }
        }
        delInfo |= result.delInfo;
        pos = result.pos;
      }
      return simple ? pos : new MapResult(pos, delInfo, null);
    }
  };
  var stepsByID = /* @__PURE__ */ Object.create(null);
  var Step = class {
    /**
    Get the step map that represents the changes made by this step,
    and which can be used to transform between positions in the old
    and the new document.
    */
    getMap() {
      return StepMap.empty;
    }
    /**
    Try to merge this step with another one, to be applied directly
    after it. Returns the merged step when possible, null if the
    steps can't be merged.
    */
    merge(other) {
      return null;
    }
    /**
    Deserialize a step from its JSON representation. Will call
    through to the step class' own implementation of this method.
    */
    static fromJSON(schema2, json) {
      if (!json || !json.stepType)
        throw new RangeError("Invalid input for Step.fromJSON");
      let type = stepsByID[json.stepType];
      if (!type)
        throw new RangeError(`No step type ${json.stepType} defined`);
      return type.fromJSON(schema2, json);
    }
    /**
    To be able to serialize steps to JSON, each step needs a string
    ID to attach to its JSON representation. Use this method to
    register an ID for your step classes. Try to pick something
    that's unlikely to clash with steps from other modules.
    */
    static jsonID(id, stepClass) {
      if (id in stepsByID)
        throw new RangeError("Duplicate use of step JSON ID " + id);
      stepsByID[id] = stepClass;
      stepClass.prototype.jsonID = id;
      return stepClass;
    }
  };
  var StepResult = class _StepResult {
    /**
    @internal
    */
    constructor(doc2, failed) {
      this.doc = doc2;
      this.failed = failed;
    }
    /**
    Create a successful step result.
    */
    static ok(doc2) {
      return new _StepResult(doc2, null);
    }
    /**
    Create a failed step result.
    */
    static fail(message) {
      return new _StepResult(null, message);
    }
    /**
    Call [`Node.replace`](https://prosemirror.net/docs/ref/#model.Node.replace) with the given
    arguments. Create a successful result if it succeeds, and a
    failed one if it throws a `ReplaceError`.
    */
    static fromReplace(doc2, from, to, slice) {
      try {
        return _StepResult.ok(doc2.replace(from, to, slice));
      } catch (e) {
        if (e instanceof ReplaceError)
          return _StepResult.fail(e.message);
        throw e;
      }
    }
  };
  function mapFragment(fragment, f, parent) {
    let mapped = [];
    for (let i = 0; i < fragment.childCount; i++) {
      let child = fragment.child(i);
      if (child.content.size)
        child = child.copy(mapFragment(child.content, f, child));
      if (child.isInline)
        child = f(child, parent, i);
      mapped.push(child);
    }
    return Fragment.fromArray(mapped);
  }
  var AddMarkStep = class _AddMarkStep extends Step {
    /**
    Create a mark step.
    */
    constructor(from, to, mark) {
      super();
      this.from = from;
      this.to = to;
      this.mark = mark;
    }
    apply(doc2) {
      let oldSlice = doc2.slice(this.from, this.to), $from = doc2.resolve(this.from);
      let parent = $from.node($from.sharedDepth(this.to));
      let slice = new Slice(mapFragment(oldSlice.content, (node, parent2) => {
        if (!node.isAtom || !parent2.type.allowsMarkType(this.mark.type))
          return node;
        return node.mark(this.mark.addToSet(node.marks));
      }, parent), oldSlice.openStart, oldSlice.openEnd);
      return StepResult.fromReplace(doc2, this.from, this.to, slice);
    }
    invert() {
      return new RemoveMarkStep(this.from, this.to, this.mark);
    }
    map(mapping) {
      let from = mapping.mapResult(this.from, 1), to = mapping.mapResult(this.to, -1);
      if (from.deleted && to.deleted || from.pos >= to.pos)
        return null;
      return new _AddMarkStep(from.pos, to.pos, this.mark);
    }
    merge(other) {
      if (other instanceof _AddMarkStep && other.mark.eq(this.mark) && this.from <= other.to && this.to >= other.from)
        return new _AddMarkStep(Math.min(this.from, other.from), Math.max(this.to, other.to), this.mark);
      return null;
    }
    toJSON() {
      return {
        stepType: "addMark",
        mark: this.mark.toJSON(),
        from: this.from,
        to: this.to
      };
    }
    /**
    @internal
    */
    static fromJSON(schema2, json) {
      if (typeof json.from != "number" || typeof json.to != "number")
        throw new RangeError("Invalid input for AddMarkStep.fromJSON");
      return new _AddMarkStep(json.from, json.to, schema2.markFromJSON(json.mark));
    }
  };
  Step.jsonID("addMark", AddMarkStep);
  var RemoveMarkStep = class _RemoveMarkStep extends Step {
    /**
    Create a mark-removing step.
    */
    constructor(from, to, mark) {
      super();
      this.from = from;
      this.to = to;
      this.mark = mark;
    }
    apply(doc2) {
      let oldSlice = doc2.slice(this.from, this.to);
      let slice = new Slice(mapFragment(oldSlice.content, (node) => {
        return node.mark(this.mark.removeFromSet(node.marks));
      }, doc2), oldSlice.openStart, oldSlice.openEnd);
      return StepResult.fromReplace(doc2, this.from, this.to, slice);
    }
    invert() {
      return new AddMarkStep(this.from, this.to, this.mark);
    }
    map(mapping) {
      let from = mapping.mapResult(this.from, 1), to = mapping.mapResult(this.to, -1);
      if (from.deleted && to.deleted || from.pos >= to.pos)
        return null;
      return new _RemoveMarkStep(from.pos, to.pos, this.mark);
    }
    merge(other) {
      if (other instanceof _RemoveMarkStep && other.mark.eq(this.mark) && this.from <= other.to && this.to >= other.from)
        return new _RemoveMarkStep(Math.min(this.from, other.from), Math.max(this.to, other.to), this.mark);
      return null;
    }
    toJSON() {
      return {
        stepType: "removeMark",
        mark: this.mark.toJSON(),
        from: this.from,
        to: this.to
      };
    }
    /**
    @internal
    */
    static fromJSON(schema2, json) {
      if (typeof json.from != "number" || typeof json.to != "number")
        throw new RangeError("Invalid input for RemoveMarkStep.fromJSON");
      return new _RemoveMarkStep(json.from, json.to, schema2.markFromJSON(json.mark));
    }
  };
  Step.jsonID("removeMark", RemoveMarkStep);
  var AddNodeMarkStep = class _AddNodeMarkStep extends Step {
    /**
    Create a node mark step.
    */
    constructor(pos, mark) {
      super();
      this.pos = pos;
      this.mark = mark;
    }
    apply(doc2) {
      let node = doc2.nodeAt(this.pos);
      if (!node)
        return StepResult.fail("No node at mark step's position");
      let updated = node.type.create(node.attrs, null, this.mark.addToSet(node.marks));
      return StepResult.fromReplace(doc2, this.pos, this.pos + 1, new Slice(Fragment.from(updated), 0, node.isLeaf ? 0 : 1));
    }
    invert(doc2) {
      let node = doc2.nodeAt(this.pos);
      if (node) {
        let newSet = this.mark.addToSet(node.marks);
        if (newSet.length == node.marks.length) {
          for (let i = 0; i < node.marks.length; i++)
            if (!node.marks[i].isInSet(newSet))
              return new _AddNodeMarkStep(this.pos, node.marks[i]);
          return new _AddNodeMarkStep(this.pos, this.mark);
        }
      }
      return new RemoveNodeMarkStep(this.pos, this.mark);
    }
    map(mapping) {
      let pos = mapping.mapResult(this.pos, 1);
      return pos.deletedAfter ? null : new _AddNodeMarkStep(pos.pos, this.mark);
    }
    toJSON() {
      return { stepType: "addNodeMark", pos: this.pos, mark: this.mark.toJSON() };
    }
    /**
    @internal
    */
    static fromJSON(schema2, json) {
      if (typeof json.pos != "number")
        throw new RangeError("Invalid input for AddNodeMarkStep.fromJSON");
      return new _AddNodeMarkStep(json.pos, schema2.markFromJSON(json.mark));
    }
  };
  Step.jsonID("addNodeMark", AddNodeMarkStep);
  var RemoveNodeMarkStep = class _RemoveNodeMarkStep extends Step {
    /**
    Create a mark-removing step.
    */
    constructor(pos, mark) {
      super();
      this.pos = pos;
      this.mark = mark;
    }
    apply(doc2) {
      let node = doc2.nodeAt(this.pos);
      if (!node)
        return StepResult.fail("No node at mark step's position");
      let updated = node.type.create(node.attrs, null, this.mark.removeFromSet(node.marks));
      return StepResult.fromReplace(doc2, this.pos, this.pos + 1, new Slice(Fragment.from(updated), 0, node.isLeaf ? 0 : 1));
    }
    invert(doc2) {
      let node = doc2.nodeAt(this.pos);
      if (!node || !this.mark.isInSet(node.marks))
        return this;
      return new AddNodeMarkStep(this.pos, this.mark);
    }
    map(mapping) {
      let pos = mapping.mapResult(this.pos, 1);
      return pos.deletedAfter ? null : new _RemoveNodeMarkStep(pos.pos, this.mark);
    }
    toJSON() {
      return { stepType: "removeNodeMark", pos: this.pos, mark: this.mark.toJSON() };
    }
    /**
    @internal
    */
    static fromJSON(schema2, json) {
      if (typeof json.pos != "number")
        throw new RangeError("Invalid input for RemoveNodeMarkStep.fromJSON");
      return new _RemoveNodeMarkStep(json.pos, schema2.markFromJSON(json.mark));
    }
  };
  Step.jsonID("removeNodeMark", RemoveNodeMarkStep);
  var ReplaceStep = class _ReplaceStep extends Step {
    /**
    The given `slice` should fit the 'gap' between `from` and
    `to`—the depths must line up, and the surrounding nodes must be
    able to be joined with the open sides of the slice. When
    `structure` is true, the step will fail if the content between
    from and to is not just a sequence of closing and then opening
    tokens (this is to guard against rebased replace steps
    overwriting something they weren't supposed to).
    */
    constructor(from, to, slice, structure = false) {
      super();
      this.from = from;
      this.to = to;
      this.slice = slice;
      this.structure = structure;
    }
    apply(doc2) {
      if (this.structure && contentBetween(doc2, this.from, this.to))
        return StepResult.fail("Structure replace would overwrite content");
      return StepResult.fromReplace(doc2, this.from, this.to, this.slice);
    }
    getMap() {
      return new StepMap([this.from, this.to - this.from, this.slice.size]);
    }
    invert(doc2) {
      return new _ReplaceStep(this.from, this.from + this.slice.size, doc2.slice(this.from, this.to));
    }
    map(mapping) {
      let to = mapping.mapResult(this.to, -1);
      let from = this.from == this.to && _ReplaceStep.MAP_BIAS < 0 ? to : mapping.mapResult(this.from, 1);
      if (from.deletedAcross && to.deletedAcross)
        return null;
      return new _ReplaceStep(from.pos, Math.max(from.pos, to.pos), this.slice, this.structure);
    }
    merge(other) {
      if (!(other instanceof _ReplaceStep) || other.structure || this.structure)
        return null;
      if (this.from + this.slice.size == other.from && !this.slice.openEnd && !other.slice.openStart) {
        let slice = this.slice.size + other.slice.size == 0 ? Slice.empty : new Slice(this.slice.content.append(other.slice.content), this.slice.openStart, other.slice.openEnd);
        return new _ReplaceStep(this.from, this.to + (other.to - other.from), slice, this.structure);
      } else if (other.to == this.from && !this.slice.openStart && !other.slice.openEnd) {
        let slice = this.slice.size + other.slice.size == 0 ? Slice.empty : new Slice(other.slice.content.append(this.slice.content), other.slice.openStart, this.slice.openEnd);
        return new _ReplaceStep(other.from, this.to, slice, this.structure);
      } else {
        return null;
      }
    }
    toJSON() {
      let json = { stepType: "replace", from: this.from, to: this.to };
      if (this.slice.size)
        json.slice = this.slice.toJSON();
      if (this.structure)
        json.structure = true;
      return json;
    }
    /**
    @internal
    */
    static fromJSON(schema2, json) {
      if (typeof json.from != "number" || typeof json.to != "number")
        throw new RangeError("Invalid input for ReplaceStep.fromJSON");
      return new _ReplaceStep(json.from, json.to, Slice.fromJSON(schema2, json.slice), !!json.structure);
    }
  };
  ReplaceStep.MAP_BIAS = 1;
  Step.jsonID("replace", ReplaceStep);
  var ReplaceAroundStep = class _ReplaceAroundStep extends Step {
    /**
    Create a replace-around step with the given range and gap.
    `insert` should be the point in the slice into which the content
    of the gap should be moved. `structure` has the same meaning as
    it has in the [`ReplaceStep`](https://prosemirror.net/docs/ref/#transform.ReplaceStep) class.
    */
    constructor(from, to, gapFrom, gapTo, slice, insert, structure = false) {
      super();
      this.from = from;
      this.to = to;
      this.gapFrom = gapFrom;
      this.gapTo = gapTo;
      this.slice = slice;
      this.insert = insert;
      this.structure = structure;
    }
    apply(doc2) {
      if (this.structure && (contentBetween(doc2, this.from, this.gapFrom) || contentBetween(doc2, this.gapTo, this.to)))
        return StepResult.fail("Structure gap-replace would overwrite content");
      let gap = doc2.slice(this.gapFrom, this.gapTo);
      if (gap.openStart || gap.openEnd)
        return StepResult.fail("Gap is not a flat range");
      let inserted = this.slice.insertAt(this.insert, gap.content);
      if (!inserted)
        return StepResult.fail("Content does not fit in gap");
      return StepResult.fromReplace(doc2, this.from, this.to, inserted);
    }
    getMap() {
      return new StepMap([
        this.from,
        this.gapFrom - this.from,
        this.insert,
        this.gapTo,
        this.to - this.gapTo,
        this.slice.size - this.insert
      ]);
    }
    invert(doc2) {
      let gap = this.gapTo - this.gapFrom;
      return new _ReplaceAroundStep(this.from, this.from + this.slice.size + gap, this.from + this.insert, this.from + this.insert + gap, doc2.slice(this.from, this.to).removeBetween(this.gapFrom - this.from, this.gapTo - this.from), this.gapFrom - this.from, this.structure);
    }
    map(mapping) {
      let from = mapping.mapResult(this.from, 1), to = mapping.mapResult(this.to, -1);
      let gapFrom = this.from == this.gapFrom ? from.pos : mapping.map(this.gapFrom, -1);
      let gapTo = this.to == this.gapTo ? to.pos : mapping.map(this.gapTo, 1);
      if (from.deletedAcross && to.deletedAcross || gapFrom < from.pos || gapTo > to.pos)
        return null;
      return new _ReplaceAroundStep(from.pos, to.pos, gapFrom, gapTo, this.slice, this.insert, this.structure);
    }
    toJSON() {
      let json = {
        stepType: "replaceAround",
        from: this.from,
        to: this.to,
        gapFrom: this.gapFrom,
        gapTo: this.gapTo,
        insert: this.insert
      };
      if (this.slice.size)
        json.slice = this.slice.toJSON();
      if (this.structure)
        json.structure = true;
      return json;
    }
    /**
    @internal
    */
    static fromJSON(schema2, json) {
      if (typeof json.from != "number" || typeof json.to != "number" || typeof json.gapFrom != "number" || typeof json.gapTo != "number" || typeof json.insert != "number")
        throw new RangeError("Invalid input for ReplaceAroundStep.fromJSON");
      return new _ReplaceAroundStep(json.from, json.to, json.gapFrom, json.gapTo, Slice.fromJSON(schema2, json.slice), json.insert, !!json.structure);
    }
  };
  Step.jsonID("replaceAround", ReplaceAroundStep);
  function contentBetween(doc2, from, to) {
    let $from = doc2.resolve(from), dist = to - from, depth = $from.depth;
    while (dist > 0 && depth > 0 && $from.indexAfter(depth) == $from.node(depth).childCount) {
      depth--;
      dist--;
    }
    if (dist > 0) {
      let next = $from.node(depth).maybeChild($from.indexAfter(depth));
      while (dist > 0) {
        if (!next || next.isLeaf)
          return true;
        next = next.firstChild;
        dist--;
      }
    }
    return false;
  }
  function addMark(tr, from, to, mark) {
    let removed = [], added = [];
    let removing, adding;
    tr.doc.nodesBetween(from, to, (node, pos, parent) => {
      if (!node.isInline)
        return;
      let marks = node.marks;
      if (!mark.isInSet(marks) && parent.type.allowsMarkType(mark.type)) {
        let start = Math.max(pos, from), end = Math.min(pos + node.nodeSize, to);
        let newSet = mark.addToSet(marks);
        for (let i = 0; i < marks.length; i++) {
          if (!marks[i].isInSet(newSet)) {
            if (removing && removing.to == start && removing.mark.eq(marks[i]))
              removing.to = end;
            else
              removed.push(removing = new RemoveMarkStep(start, end, marks[i]));
          }
        }
        if (adding && adding.to == start)
          adding.to = end;
        else
          added.push(adding = new AddMarkStep(start, end, mark));
      }
    });
    removed.forEach((s) => tr.step(s));
    added.forEach((s) => tr.step(s));
  }
  function removeMark(tr, from, to, mark) {
    let matched = [], step = 0;
    tr.doc.nodesBetween(from, to, (node, pos) => {
      if (!node.isInline)
        return;
      step++;
      let toRemove = null;
      if (mark instanceof MarkType) {
        let set = node.marks, found2;
        while (found2 = mark.isInSet(set)) {
          (toRemove || (toRemove = [])).push(found2);
          set = found2.removeFromSet(set);
        }
      } else if (mark) {
        if (mark.isInSet(node.marks))
          toRemove = [mark];
      } else {
        toRemove = node.marks;
      }
      if (toRemove && toRemove.length) {
        let end = Math.min(pos + node.nodeSize, to);
        for (let i = 0; i < toRemove.length; i++) {
          let style = toRemove[i], found2;
          for (let j = 0; j < matched.length; j++) {
            let m = matched[j];
            if (m.step == step - 1 && style.eq(matched[j].style))
              found2 = m;
          }
          if (found2) {
            found2.to = end;
            found2.step = step;
          } else {
            matched.push({ style, from: Math.max(pos, from), to: end, step });
          }
        }
      }
    });
    matched.forEach((m) => tr.step(new RemoveMarkStep(m.from, m.to, m.style)));
  }
  function clearIncompatible(tr, pos, parentType, match = parentType.contentMatch, clearNewlines = true) {
    let node = tr.doc.nodeAt(pos);
    let replSteps = [], cur = pos + 1;
    for (let i = 0; i < node.childCount; i++) {
      let child = node.child(i), end = cur + child.nodeSize;
      let allowed = match.matchType(child.type);
      if (!allowed) {
        replSteps.push(new ReplaceStep(cur, end, Slice.empty));
      } else {
        match = allowed;
        for (let j = 0; j < child.marks.length; j++)
          if (!parentType.allowsMarkType(child.marks[j].type))
            tr.step(new RemoveMarkStep(cur, end, child.marks[j]));
        if (clearNewlines && child.isText && parentType.whitespace != "pre") {
          let m, newline = /\r?\n|\r/g, slice;
          while (m = newline.exec(child.text)) {
            if (!slice)
              slice = new Slice(Fragment.from(parentType.schema.text(" ", parentType.allowedMarks(child.marks))), 0, 0);
            replSteps.push(new ReplaceStep(cur + m.index, cur + m.index + m[0].length, slice));
          }
        }
      }
      cur = end;
    }
    if (!match.validEnd) {
      let fill = match.fillBefore(Fragment.empty, true);
      tr.replace(cur, cur, new Slice(fill, 0, 0));
    }
    for (let i = replSteps.length - 1; i >= 0; i--)
      tr.step(replSteps[i]);
  }
  function canCut(node, start, end) {
    return (start == 0 || node.canReplace(start, node.childCount)) && (end == node.childCount || node.canReplace(0, end));
  }
  function liftTarget(range) {
    let parent = range.parent;
    let content = parent.content.cutByIndex(range.startIndex, range.endIndex);
    for (let depth = range.depth, contentBefore = 0, contentAfter = 0; ; --depth) {
      let node = range.$from.node(depth);
      let index = range.$from.index(depth) + contentBefore, endIndex = range.$to.indexAfter(depth) - contentAfter;
      if (depth < range.depth && node.canReplace(index, endIndex, content))
        return depth;
      if (depth == 0 || node.type.spec.isolating || !canCut(node, index, endIndex))
        break;
      if (index)
        contentBefore = 1;
      if (endIndex < node.childCount)
        contentAfter = 1;
    }
    return null;
  }
  function lift(tr, range, target) {
    let { $from, $to, depth } = range;
    let gapStart = $from.before(depth + 1), gapEnd = $to.after(depth + 1);
    let start = gapStart, end = gapEnd;
    let before = Fragment.empty, openStart = 0;
    for (let d = depth, splitting = false; d > target; d--)
      if (splitting || $from.index(d) > 0) {
        splitting = true;
        before = Fragment.from($from.node(d).copy(before));
        openStart++;
      } else {
        start--;
      }
    let after = Fragment.empty, openEnd = 0;
    for (let d = depth, splitting = false; d > target; d--)
      if (splitting || $to.after(d + 1) < $to.end(d)) {
        splitting = true;
        after = Fragment.from($to.node(d).copy(after));
        openEnd++;
      } else {
        end++;
      }
    tr.step(new ReplaceAroundStep(start, end, gapStart, gapEnd, new Slice(before.append(after), openStart, openEnd), before.size - openStart, true));
  }
  function findWrapping(range, nodeType, attrs = null, innerRange = range) {
    let around = findWrappingOutside(range, nodeType);
    let inner = around && findWrappingInside(innerRange, nodeType);
    if (!inner)
      return null;
    return around.map(withAttrs).concat({ type: nodeType, attrs }).concat(inner.map(withAttrs));
  }
  function withAttrs(type) {
    return { type, attrs: null };
  }
  function findWrappingOutside(range, type) {
    let { parent, startIndex, endIndex } = range;
    let around = parent.contentMatchAt(startIndex).findWrapping(type);
    if (!around)
      return null;
    let outer = around.length ? around[0] : type;
    return parent.canReplaceWith(startIndex, endIndex, outer) ? around : null;
  }
  function findWrappingInside(range, type) {
    let { parent, startIndex, endIndex } = range;
    let inner = parent.child(startIndex);
    let inside = type.contentMatch.findWrapping(inner.type);
    if (!inside)
      return null;
    let lastType = inside.length ? inside[inside.length - 1] : type;
    let innerMatch = lastType.contentMatch;
    for (let i = startIndex; innerMatch && i < endIndex; i++)
      innerMatch = innerMatch.matchType(parent.child(i).type);
    if (!innerMatch || !innerMatch.validEnd)
      return null;
    return inside;
  }
  function wrap(tr, range, wrappers) {
    let content = Fragment.empty;
    for (let i = wrappers.length - 1; i >= 0; i--) {
      if (content.size) {
        let match = wrappers[i].type.contentMatch.matchFragment(content);
        if (!match || !match.validEnd)
          throw new RangeError("Wrapper type given to Transform.wrap does not form valid content of its parent wrapper");
      }
      content = Fragment.from(wrappers[i].type.create(wrappers[i].attrs, content));
    }
    let start = range.start, end = range.end;
    tr.step(new ReplaceAroundStep(start, end, start, end, new Slice(content, 0, 0), wrappers.length, true));
  }
  function setBlockType(tr, from, to, type, attrs) {
    if (!type.isTextblock)
      throw new RangeError("Type given to setBlockType should be a textblock");
    let mapFrom = tr.steps.length;
    tr.doc.nodesBetween(from, to, (node, pos) => {
      let attrsHere = typeof attrs == "function" ? attrs(node) : attrs;
      if (node.isTextblock && !node.hasMarkup(type, attrsHere) && canChangeType(tr.doc, tr.mapping.slice(mapFrom).map(pos), type)) {
        let convertNewlines = null;
        if (type.schema.linebreakReplacement) {
          let pre = type.whitespace == "pre", supportLinebreak = !!type.contentMatch.matchType(type.schema.linebreakReplacement);
          if (pre && !supportLinebreak)
            convertNewlines = false;
          else if (!pre && supportLinebreak)
            convertNewlines = true;
        }
        if (convertNewlines === false)
          replaceLinebreaks(tr, node, pos, mapFrom);
        clearIncompatible(tr, tr.mapping.slice(mapFrom).map(pos, 1), type, void 0, convertNewlines === null);
        let mapping = tr.mapping.slice(mapFrom);
        let startM = mapping.map(pos, 1), endM = mapping.map(pos + node.nodeSize, 1);
        tr.step(new ReplaceAroundStep(startM, endM, startM + 1, endM - 1, new Slice(Fragment.from(type.create(attrsHere, null, node.marks)), 0, 0), 1, true));
        if (convertNewlines === true)
          replaceNewlines(tr, node, pos, mapFrom);
        return false;
      }
    });
  }
  function replaceNewlines(tr, node, pos, mapFrom) {
    node.forEach((child, offset) => {
      if (child.isText) {
        let m, newline = /\r?\n|\r/g;
        while (m = newline.exec(child.text)) {
          let start = tr.mapping.slice(mapFrom).map(pos + 1 + offset + m.index);
          tr.replaceWith(start, start + 1, node.type.schema.linebreakReplacement.create());
        }
      }
    });
  }
  function replaceLinebreaks(tr, node, pos, mapFrom) {
    node.forEach((child, offset) => {
      if (child.type == child.type.schema.linebreakReplacement) {
        let start = tr.mapping.slice(mapFrom).map(pos + 1 + offset);
        tr.replaceWith(start, start + 1, node.type.schema.text("\n"));
      }
    });
  }
  function canChangeType(doc2, pos, type) {
    let $pos = doc2.resolve(pos), index = $pos.index();
    return $pos.parent.canReplaceWith(index, index + 1, type);
  }
  function setNodeMarkup(tr, pos, type, attrs, marks) {
    let node = tr.doc.nodeAt(pos);
    if (!node)
      throw new RangeError("No node at given position");
    if (!type)
      type = node.type;
    let newNode = type.create(attrs, null, marks || node.marks);
    if (node.isLeaf)
      return tr.replaceWith(pos, pos + node.nodeSize, newNode);
    if (!type.validContent(node.content))
      throw new RangeError("Invalid content for node type " + type.name);
    tr.step(new ReplaceAroundStep(pos, pos + node.nodeSize, pos + 1, pos + node.nodeSize - 1, new Slice(Fragment.from(newNode), 0, 0), 1, true));
  }
  function canSplit(doc2, pos, depth = 1, typesAfter) {
    let $pos = doc2.resolve(pos), base = $pos.depth - depth;
    let innerType = typesAfter && typesAfter[typesAfter.length - 1] || $pos.parent;
    if (base < 0 || $pos.parent.type.spec.isolating || !$pos.parent.canReplace($pos.index(), $pos.parent.childCount) || !innerType.type.validContent($pos.parent.content.cutByIndex($pos.index(), $pos.parent.childCount)))
      return false;
    for (let d = $pos.depth - 1, i = depth - 2; d > base; d--, i--) {
      let node = $pos.node(d), index2 = $pos.index(d);
      if (node.type.spec.isolating)
        return false;
      let rest = node.content.cutByIndex(index2, node.childCount);
      let overrideChild = typesAfter && typesAfter[i + 1];
      if (overrideChild)
        rest = rest.replaceChild(0, overrideChild.type.create(overrideChild.attrs));
      let after = typesAfter && typesAfter[i] || node;
      if (!node.canReplace(index2 + 1, node.childCount) || !after.type.validContent(rest))
        return false;
    }
    let index = $pos.indexAfter(base);
    let baseType = typesAfter && typesAfter[0];
    return $pos.node(base).canReplaceWith(index, index, baseType ? baseType.type : $pos.node(base + 1).type);
  }
  function split(tr, pos, depth = 1, typesAfter) {
    let $pos = tr.doc.resolve(pos), before = Fragment.empty, after = Fragment.empty;
    for (let d = $pos.depth, e = $pos.depth - depth, i = depth - 1; d > e; d--, i--) {
      before = Fragment.from($pos.node(d).copy(before));
      let typeAfter = typesAfter && typesAfter[i];
      after = Fragment.from(typeAfter ? typeAfter.type.create(typeAfter.attrs, after) : $pos.node(d).copy(after));
    }
    tr.step(new ReplaceStep(pos, pos, new Slice(before.append(after), depth, depth), true));
  }
  function canJoin(doc2, pos) {
    let $pos = doc2.resolve(pos), index = $pos.index();
    return joinable2($pos.nodeBefore, $pos.nodeAfter) && $pos.parent.canReplace(index, index + 1);
  }
  function canAppendWithSubstitutedLinebreaks(a, b) {
    if (!b.content.size)
      a.type.compatibleContent(b.type);
    let match = a.contentMatchAt(a.childCount);
    let { linebreakReplacement } = a.type.schema;
    for (let i = 0; i < b.childCount; i++) {
      let child = b.child(i);
      let type = child.type == linebreakReplacement ? a.type.schema.nodes.text : child.type;
      match = match.matchType(type);
      if (!match)
        return false;
      if (!a.type.allowsMarks(child.marks))
        return false;
    }
    return match.validEnd;
  }
  function joinable2(a, b) {
    return !!(a && b && !a.isLeaf && canAppendWithSubstitutedLinebreaks(a, b));
  }
  function join(tr, pos, depth) {
    let convertNewlines = null;
    let { linebreakReplacement } = tr.doc.type.schema;
    let $before = tr.doc.resolve(pos - depth), beforeType = $before.node().type;
    if (linebreakReplacement && beforeType.inlineContent) {
      let pre = beforeType.whitespace == "pre";
      let supportLinebreak = !!beforeType.contentMatch.matchType(linebreakReplacement);
      if (pre && !supportLinebreak)
        convertNewlines = false;
      else if (!pre && supportLinebreak)
        convertNewlines = true;
    }
    let mapFrom = tr.steps.length;
    if (convertNewlines === false) {
      let $after = tr.doc.resolve(pos + depth);
      replaceLinebreaks(tr, $after.node(), $after.before(), mapFrom);
    }
    if (beforeType.inlineContent)
      clearIncompatible(tr, pos + depth - 1, beforeType, $before.node().contentMatchAt($before.index()), convertNewlines == null);
    let mapping = tr.mapping.slice(mapFrom), start = mapping.map(pos - depth);
    tr.step(new ReplaceStep(start, mapping.map(pos + depth, -1), Slice.empty, true));
    if (convertNewlines === true) {
      let $full = tr.doc.resolve(start);
      replaceNewlines(tr, $full.node(), $full.before(), tr.steps.length);
    }
    return tr;
  }
  function insertPoint(doc2, pos, nodeType) {
    let $pos = doc2.resolve(pos);
    if ($pos.parent.canReplaceWith($pos.index(), $pos.index(), nodeType))
      return pos;
    if ($pos.parentOffset == 0)
      for (let d = $pos.depth - 1; d >= 0; d--) {
        let index = $pos.index(d);
        if ($pos.node(d).canReplaceWith(index, index, nodeType))
          return $pos.before(d + 1);
        if (index > 0)
          return null;
      }
    if ($pos.parentOffset == $pos.parent.content.size)
      for (let d = $pos.depth - 1; d >= 0; d--) {
        let index = $pos.indexAfter(d);
        if ($pos.node(d).canReplaceWith(index, index, nodeType))
          return $pos.after(d + 1);
        if (index < $pos.node(d).childCount)
          return null;
      }
    return null;
  }
  function replaceStep(doc2, from, to = from, slice = Slice.empty) {
    if (from == to && !slice.size)
      return null;
    let $from = doc2.resolve(from), $to = doc2.resolve(to);
    if (fitsTrivially($from, $to, slice))
      return new ReplaceStep(from, to, slice);
    return new Fitter($from, $to, slice).fit();
  }
  function fitsTrivially($from, $to, slice) {
    return !slice.openStart && !slice.openEnd && $from.start() == $to.start() && $from.parent.canReplace($from.index(), $to.index(), slice.content);
  }
  var Fitter = class {
    constructor($from, $to, unplaced) {
      this.$from = $from;
      this.$to = $to;
      this.unplaced = unplaced;
      this.frontier = [];
      this.placed = Fragment.empty;
      for (let i = 0; i <= $from.depth; i++) {
        let node = $from.node(i);
        this.frontier.push({
          type: node.type,
          match: node.contentMatchAt($from.indexAfter(i))
        });
      }
      for (let i = $from.depth; i > 0; i--)
        this.placed = Fragment.from($from.node(i).copy(this.placed));
    }
    get depth() {
      return this.frontier.length - 1;
    }
    fit() {
      while (this.unplaced.size) {
        let fit = this.findFittable();
        if (fit)
          this.placeNodes(fit);
        else
          this.openMore() || this.dropNode();
      }
      let moveInline = this.mustMoveInline(), placedSize = this.placed.size - this.depth - this.$from.depth;
      let $from = this.$from, $to = this.close(moveInline < 0 ? this.$to : $from.doc.resolve(moveInline));
      if (!$to)
        return null;
      let content = this.placed, openStart = $from.depth, openEnd = $to.depth;
      while (openStart && openEnd && content.childCount == 1) {
        content = content.firstChild.content;
        openStart--;
        openEnd--;
      }
      let slice = new Slice(content, openStart, openEnd);
      if (moveInline > -1)
        return new ReplaceAroundStep($from.pos, moveInline, this.$to.pos, this.$to.end(), slice, placedSize);
      if (slice.size || $from.pos != this.$to.pos)
        return new ReplaceStep($from.pos, $to.pos, slice);
      return null;
    }
    // Find a position on the start spine of `this.unplaced` that has
    // content that can be moved somewhere on the frontier. Returns two
    // depths, one for the slice and one for the frontier.
    findFittable() {
      let startDepth = this.unplaced.openStart;
      for (let cur = this.unplaced.content, d = 0, openEnd = this.unplaced.openEnd; d < startDepth; d++) {
        let node = cur.firstChild;
        if (cur.childCount > 1)
          openEnd = 0;
        if (node.type.spec.isolating && openEnd <= d) {
          startDepth = d;
          break;
        }
        cur = node.content;
      }
      for (let pass = 1; pass <= 2; pass++) {
        for (let sliceDepth = pass == 1 ? startDepth : this.unplaced.openStart; sliceDepth >= 0; sliceDepth--) {
          let fragment, parent = null;
          if (sliceDepth) {
            parent = contentAt(this.unplaced.content, sliceDepth - 1).firstChild;
            fragment = parent.content;
          } else {
            fragment = this.unplaced.content;
          }
          let first = fragment.firstChild;
          for (let frontierDepth = this.depth; frontierDepth >= 0; frontierDepth--) {
            let { type, match } = this.frontier[frontierDepth], wrap2, inject = null;
            if (pass == 1 && (first ? match.matchType(first.type) || (inject = match.fillBefore(Fragment.from(first), false)) : parent && type.compatibleContent(parent.type)))
              return { sliceDepth, frontierDepth, parent, inject };
            else if (pass == 2 && first && (wrap2 = match.findWrapping(first.type)))
              return { sliceDepth, frontierDepth, parent, wrap: wrap2 };
            if (parent && match.matchType(parent.type))
              break;
          }
        }
      }
    }
    openMore() {
      let { content, openStart, openEnd } = this.unplaced;
      if (maxOpen(content, -1) <= openStart)
        return false;
      if (this.unplaced.size > 1 && maxOpen(content, 1) > openEnd)
        openEnd++;
      this.unplaced = new Slice(content, openStart + 1, openEnd);
      return true;
    }
    dropNode() {
      let { content, openStart, openEnd } = this.unplaced;
      let inner = contentAt(content, openStart);
      if (inner.childCount <= 1 && openStart > 0) {
        let openAtEnd = content.size - openStart <= openStart + inner.size;
        this.unplaced = new Slice(dropFromFragment(content, openStart - 1, 1), openStart - 1, openAtEnd ? openStart - 1 : openEnd);
      } else {
        this.unplaced = new Slice(dropFromFragment(content, openStart, 1), openStart, openEnd);
      }
    }
    // Move content from the unplaced slice at `sliceDepth` to the
    // frontier node at `frontierDepth`. Close that frontier node when
    // applicable.
    placeNodes({ sliceDepth, frontierDepth, parent, inject, wrap: wrap2 }) {
      while (this.depth > frontierDepth)
        this.closeFrontierNode();
      if (wrap2)
        for (let i = 0; i < wrap2.length; i++)
          this.openFrontierNode(wrap2[i]);
      let slice = this.unplaced, fragment = parent ? parent.content : slice.content;
      let openStart = slice.openStart - sliceDepth;
      let taken = 0, add = [];
      let { match, type } = this.frontier[frontierDepth];
      if (inject) {
        for (let i = 0; i < inject.childCount; i++)
          add.push(inject.child(i));
        match = match.matchFragment(inject);
      }
      let openEndCount = fragment.size + sliceDepth - (slice.content.size - slice.openEnd);
      while (taken < fragment.childCount) {
        let next = fragment.child(taken), matches = match.matchType(next.type);
        if (!matches)
          break;
        taken++;
        if (taken > 1 || openStart == 0 || next.content.size) {
          match = matches;
          add.push(closeNodeStart(next.mark(type.allowedMarks(next.marks)), taken == 1 ? openStart : 0, taken == fragment.childCount ? openEndCount : -1));
        }
      }
      let toEnd = taken == fragment.childCount;
      if (!toEnd)
        openEndCount = -1;
      this.placed = addToFragment(this.placed, frontierDepth, Fragment.from(add));
      this.frontier[frontierDepth].match = match;
      if (toEnd && openEndCount < 0 && parent && parent.type == this.frontier[this.depth].type && this.frontier.length > 1)
        this.closeFrontierNode();
      for (let i = 0, cur = fragment; i < openEndCount; i++) {
        let node = cur.lastChild;
        this.frontier.push({ type: node.type, match: node.contentMatchAt(node.childCount) });
        cur = node.content;
      }
      this.unplaced = !toEnd ? new Slice(dropFromFragment(slice.content, sliceDepth, taken), slice.openStart, slice.openEnd) : sliceDepth == 0 ? Slice.empty : new Slice(dropFromFragment(slice.content, sliceDepth - 1, 1), sliceDepth - 1, openEndCount < 0 ? slice.openEnd : sliceDepth - 1);
    }
    mustMoveInline() {
      if (!this.$to.parent.isTextblock)
        return -1;
      let top = this.frontier[this.depth], level;
      if (!top.type.isTextblock || !contentAfterFits(this.$to, this.$to.depth, top.type, top.match, false) || this.$to.depth == this.depth && (level = this.findCloseLevel(this.$to)) && level.depth == this.depth)
        return -1;
      let { depth } = this.$to, after = this.$to.after(depth);
      while (depth > 1 && after == this.$to.end(--depth))
        ++after;
      return after;
    }
    findCloseLevel($to) {
      scan: for (let i = Math.min(this.depth, $to.depth); i >= 0; i--) {
        let { match, type } = this.frontier[i];
        let dropInner = i < $to.depth && $to.end(i + 1) == $to.pos + ($to.depth - (i + 1));
        let fit = contentAfterFits($to, i, type, match, dropInner);
        if (!fit)
          continue;
        for (let d = i - 1; d >= 0; d--) {
          let { match: match2, type: type2 } = this.frontier[d];
          let matches = contentAfterFits($to, d, type2, match2, true);
          if (!matches || matches.childCount)
            continue scan;
        }
        return { depth: i, fit, move: dropInner ? $to.doc.resolve($to.after(i + 1)) : $to };
      }
    }
    close($to) {
      let close2 = this.findCloseLevel($to);
      if (!close2)
        return null;
      while (this.depth > close2.depth)
        this.closeFrontierNode();
      if (close2.fit.childCount)
        this.placed = addToFragment(this.placed, close2.depth, close2.fit);
      $to = close2.move;
      for (let d = close2.depth + 1; d <= $to.depth; d++) {
        let node = $to.node(d), add = node.type.contentMatch.fillBefore(node.content, true, $to.index(d));
        this.openFrontierNode(node.type, node.attrs, add);
      }
      return $to;
    }
    openFrontierNode(type, attrs = null, content) {
      let top = this.frontier[this.depth];
      top.match = top.match.matchType(type);
      this.placed = addToFragment(this.placed, this.depth, Fragment.from(type.create(attrs, content)));
      this.frontier.push({ type, match: type.contentMatch });
    }
    closeFrontierNode() {
      let open = this.frontier.pop();
      let add = open.match.fillBefore(Fragment.empty, true);
      if (add.childCount)
        this.placed = addToFragment(this.placed, this.frontier.length, add);
    }
  };
  function dropFromFragment(fragment, depth, count) {
    if (depth == 0)
      return fragment.cutByIndex(count, fragment.childCount);
    return fragment.replaceChild(0, fragment.firstChild.copy(dropFromFragment(fragment.firstChild.content, depth - 1, count)));
  }
  function addToFragment(fragment, depth, content) {
    if (depth == 0)
      return fragment.append(content);
    return fragment.replaceChild(fragment.childCount - 1, fragment.lastChild.copy(addToFragment(fragment.lastChild.content, depth - 1, content)));
  }
  function contentAt(fragment, depth) {
    for (let i = 0; i < depth; i++)
      fragment = fragment.firstChild.content;
    return fragment;
  }
  function closeNodeStart(node, openStart, openEnd) {
    if (openStart <= 0)
      return node;
    let frag = node.content;
    if (openStart > 1)
      frag = frag.replaceChild(0, closeNodeStart(frag.firstChild, openStart - 1, frag.childCount == 1 ? openEnd - 1 : 0));
    if (openStart > 0) {
      frag = node.type.contentMatch.fillBefore(frag).append(frag);
      if (openEnd <= 0)
        frag = frag.append(node.type.contentMatch.matchFragment(frag).fillBefore(Fragment.empty, true));
    }
    return node.copy(frag);
  }
  function contentAfterFits($to, depth, type, match, open) {
    let node = $to.node(depth), index = open ? $to.indexAfter(depth) : $to.index(depth);
    if (index == node.childCount && !type.compatibleContent(node.type))
      return null;
    let fit = match.fillBefore(node.content, true, index);
    return fit && !invalidMarks(type, node.content, index) ? fit : null;
  }
  function invalidMarks(type, fragment, start) {
    for (let i = start; i < fragment.childCount; i++)
      if (!type.allowsMarks(fragment.child(i).marks))
        return true;
    return false;
  }
  function definesContent(type) {
    return type.spec.defining || type.spec.definingForContent;
  }
  function maxOpen(frag, side) {
    for (let count = 0; ; count++) {
      let ch = side < 0 ? frag.firstChild : frag.lastChild;
      if (!ch || ch.isAtom)
        return count;
      frag = ch.content;
    }
  }
  function replaceRange(tr, from, to, slice) {
    if (!slice.size)
      return tr.deleteRange(from, to);
    let $from = tr.doc.resolve(from), $to = tr.doc.resolve(to);
    if (fitsTrivially($from, $to, slice))
      return tr.step(new ReplaceStep(from, to, slice));
    let targetDepths = coveredDepths($from, $to);
    if (targetDepths[targetDepths.length - 1] == 0)
      targetDepths.pop();
    let preferredTarget = -($from.depth + 1);
    targetDepths.unshift(preferredTarget);
    for (let d = $from.depth, pos = $from.pos - 1; d > 0; d--, pos--) {
      let spec = $from.node(d).type.spec;
      if (spec.defining || spec.definingAsContext || spec.isolating)
        break;
      if (targetDepths.indexOf(d) > -1)
        preferredTarget = d;
      else if ($from.before(d) == pos)
        targetDepths.splice(1, 0, -d);
    }
    let preferredTargetIndex = targetDepths.indexOf(preferredTarget);
    let leftNodes = [], preferredDepth = slice.openStart;
    for (let content = slice.content, i = 0; ; i++) {
      let node = content.firstChild;
      leftNodes.push(node);
      if (i == slice.openStart)
        break;
      content = node.content;
    }
    for (let d = preferredDepth - 1; d >= 0; d--) {
      let leftNode = leftNodes[d], def = definesContent(leftNode.type);
      if (def && !leftNode.sameMarkup($from.node(Math.abs(preferredTarget) - 1)))
        preferredDepth = d;
      else if (def || !leftNode.type.isTextblock)
        break;
    }
    for (let j = slice.openStart; j >= 0; j--) {
      let openDepth = (j + preferredDepth + 1) % (slice.openStart + 1);
      let insert = leftNodes[openDepth];
      if (!insert)
        continue;
      for (let i = 0; i < targetDepths.length; i++) {
        let targetDepth = targetDepths[(i + preferredTargetIndex) % targetDepths.length], expand = true;
        if (targetDepth < 0) {
          expand = false;
          targetDepth = -targetDepth;
        }
        let parent = $from.node(targetDepth - 1), index = $from.index(targetDepth - 1);
        if (parent.canReplaceWith(index, index, insert.type, insert.marks))
          return tr.replace($from.before(targetDepth), expand ? $to.after(targetDepth) : to, new Slice(closeFragment(slice.content, 0, slice.openStart, openDepth), openDepth, slice.openEnd));
      }
    }
    let startSteps = tr.steps.length;
    for (let i = targetDepths.length - 1; i >= 0; i--) {
      tr.replace(from, to, slice);
      if (tr.steps.length > startSteps)
        break;
      let depth = targetDepths[i];
      if (depth < 0)
        continue;
      from = $from.before(depth);
      to = $to.after(depth);
    }
  }
  function closeFragment(fragment, depth, oldOpen, newOpen, parent) {
    if (depth < oldOpen) {
      let first = fragment.firstChild;
      fragment = fragment.replaceChild(0, first.copy(closeFragment(first.content, depth + 1, oldOpen, newOpen, first)));
    }
    if (depth > newOpen) {
      let match = parent.contentMatchAt(0);
      let start = match.fillBefore(fragment).append(fragment);
      fragment = start.append(match.matchFragment(start).fillBefore(Fragment.empty, true));
    }
    return fragment;
  }
  function replaceRangeWith(tr, from, to, node) {
    if (!node.isInline && from == to && tr.doc.resolve(from).parent.content.size) {
      let point = insertPoint(tr.doc, from, node.type);
      if (point != null)
        from = to = point;
    }
    tr.replaceRange(from, to, new Slice(Fragment.from(node), 0, 0));
  }
  function deleteRange(tr, from, to) {
    let $from = tr.doc.resolve(from), $to = tr.doc.resolve(to);
    if ($from.parent.isTextblock && $to.parent.isTextblock && $from.start() != $to.start() && $from.parentOffset == 0 && $to.parentOffset == 0) {
      let shared = $from.sharedDepth(to), isolated = false;
      for (let d = $from.depth; d > shared; d--)
        if ($from.node(d).type.spec.isolating)
          isolated = true;
      for (let d = $to.depth; d > shared; d--)
        if ($to.node(d).type.spec.isolating)
          isolated = true;
      if (!isolated) {
        for (let d = $from.depth; d > 0 && from == $from.start(d); d--)
          from = $from.before(d);
        for (let d = $to.depth; d > 0 && to == $to.start(d); d--)
          to = $to.before(d);
        $from = tr.doc.resolve(from);
        $to = tr.doc.resolve(to);
      }
    }
    let covered = coveredDepths($from, $to);
    for (let i = 0; i < covered.length; i++) {
      let depth = covered[i], last = i == covered.length - 1;
      if (last && depth == 0 || $from.node(depth).type.contentMatch.validEnd)
        return tr.delete($from.start(depth), $to.end(depth));
      if (depth > 0 && (last || $from.node(depth - 1).canReplace($from.index(depth - 1), $to.indexAfter(depth - 1))))
        return tr.delete($from.before(depth), $to.after(depth));
    }
    for (let d = 1; d <= $from.depth && d <= $to.depth; d++) {
      if (from - $from.start(d) == $from.depth - d && to > $from.end(d) && $to.end(d) - to != $to.depth - d && $from.start(d - 1) == $to.start(d - 1) && $from.node(d - 1).canReplace($from.index(d - 1), $to.index(d - 1)))
        return tr.delete($from.before(d), to);
    }
    tr.delete(from, to);
  }
  function coveredDepths($from, $to) {
    let result = [], minDepth = Math.min($from.depth, $to.depth);
    for (let d = minDepth; d >= 0; d--) {
      let start = $from.start(d);
      if (start < $from.pos - ($from.depth - d) || $to.end(d) > $to.pos + ($to.depth - d) || $from.node(d).type.spec.isolating || $to.node(d).type.spec.isolating)
        break;
      if (start == $to.start(d) || d == $from.depth && d == $to.depth && $from.parent.inlineContent && $to.parent.inlineContent && d && $to.start(d - 1) == start - 1)
        result.push(d);
    }
    return result;
  }
  var AttrStep = class _AttrStep extends Step {
    /**
    Construct an attribute step.
    */
    constructor(pos, attr, value) {
      super();
      this.pos = pos;
      this.attr = attr;
      this.value = value;
    }
    apply(doc2) {
      let node = doc2.nodeAt(this.pos);
      if (!node)
        return StepResult.fail("No node at attribute step's position");
      let attrs = /* @__PURE__ */ Object.create(null);
      for (let name in node.attrs)
        attrs[name] = node.attrs[name];
      attrs[this.attr] = this.value;
      let updated = node.type.create(attrs, null, node.marks);
      return StepResult.fromReplace(doc2, this.pos, this.pos + 1, new Slice(Fragment.from(updated), 0, node.isLeaf ? 0 : 1));
    }
    getMap() {
      return StepMap.empty;
    }
    invert(doc2) {
      return new _AttrStep(this.pos, this.attr, doc2.nodeAt(this.pos).attrs[this.attr]);
    }
    map(mapping) {
      let pos = mapping.mapResult(this.pos, 1);
      return pos.deletedAfter ? null : new _AttrStep(pos.pos, this.attr, this.value);
    }
    toJSON() {
      return { stepType: "attr", pos: this.pos, attr: this.attr, value: this.value };
    }
    static fromJSON(schema2, json) {
      if (typeof json.pos != "number" || typeof json.attr != "string")
        throw new RangeError("Invalid input for AttrStep.fromJSON");
      return new _AttrStep(json.pos, json.attr, json.value);
    }
  };
  Step.jsonID("attr", AttrStep);
  var DocAttrStep = class _DocAttrStep extends Step {
    /**
    Construct an attribute step.
    */
    constructor(attr, value) {
      super();
      this.attr = attr;
      this.value = value;
    }
    apply(doc2) {
      let attrs = /* @__PURE__ */ Object.create(null);
      for (let name in doc2.attrs)
        attrs[name] = doc2.attrs[name];
      attrs[this.attr] = this.value;
      let updated = doc2.type.create(attrs, doc2.content, doc2.marks);
      return StepResult.ok(updated);
    }
    getMap() {
      return StepMap.empty;
    }
    invert(doc2) {
      return new _DocAttrStep(this.attr, doc2.attrs[this.attr]);
    }
    map(mapping) {
      return this;
    }
    toJSON() {
      return { stepType: "docAttr", attr: this.attr, value: this.value };
    }
    static fromJSON(schema2, json) {
      if (typeof json.attr != "string")
        throw new RangeError("Invalid input for DocAttrStep.fromJSON");
      return new _DocAttrStep(json.attr, json.value);
    }
  };
  Step.jsonID("docAttr", DocAttrStep);
  var TransformError = class extends Error {
  };
  TransformError = function TransformError2(message) {
    let err = Error.call(this, message);
    err.__proto__ = TransformError2.prototype;
    return err;
  };
  TransformError.prototype = Object.create(Error.prototype);
  TransformError.prototype.constructor = TransformError;
  TransformError.prototype.name = "TransformError";
  var Transform = class {
    /**
    Create a transform that starts with the given document.
    */
    constructor(doc2) {
      this.doc = doc2;
      this.steps = [];
      this.docs = [];
      this.mapping = new Mapping();
    }
    /**
    The starting document.
    */
    get before() {
      return this.docs.length ? this.docs[0] : this.doc;
    }
    /**
    Apply a new step in this transform, saving the result. Throws an
    error when the step fails.
    */
    step(step) {
      let result = this.maybeStep(step);
      if (result.failed)
        throw new TransformError(result.failed);
      return this;
    }
    /**
    Try to apply a step in this transformation, ignoring it if it
    fails. Returns the step result.
    */
    maybeStep(step) {
      let result = step.apply(this.doc);
      if (!result.failed)
        this.addStep(step, result.doc);
      return result;
    }
    /**
    True when the document has been changed (when there are any
    steps).
    */
    get docChanged() {
      return this.steps.length > 0;
    }
    /**
    Return a single range, in post-transform document positions,
    that covers all content changed by this transform. Returns null
    if no replacements are made. Note that this will ignore changes
    that add/remove marks without replacing the underlying content.
    */
    changedRange() {
      let from = 1e9, to = -1e9;
      for (let i = 0; i < this.mapping.maps.length; i++) {
        let map = this.mapping.maps[i];
        if (i) {
          from = map.map(from, 1);
          to = map.map(to, -1);
        }
        map.forEach((_f, _t, fromB, toB) => {
          from = Math.min(from, fromB);
          to = Math.max(to, toB);
        });
      }
      return from == 1e9 ? null : { from, to };
    }
    /**
    @internal
    */
    addStep(step, doc2) {
      this.docs.push(this.doc);
      this.steps.push(step);
      this.mapping.appendMap(step.getMap());
      this.doc = doc2;
    }
    /**
    Replace the part of the document between `from` and `to` with the
    given `slice`.
    */
    replace(from, to = from, slice = Slice.empty) {
      let step = replaceStep(this.doc, from, to, slice);
      if (step)
        this.step(step);
      return this;
    }
    /**
    Replace the given range with the given content, which may be a
    fragment, node, or array of nodes.
    */
    replaceWith(from, to, content) {
      return this.replace(from, to, new Slice(Fragment.from(content), 0, 0));
    }
    /**
    Delete the content between the given positions.
    */
    delete(from, to) {
      return this.replace(from, to, Slice.empty);
    }
    /**
    Insert the given content at the given position.
    */
    insert(pos, content) {
      return this.replaceWith(pos, pos, content);
    }
    /**
    Replace a range of the document with a given slice, using
    `from`, `to`, and the slice's
    [`openStart`](https://prosemirror.net/docs/ref/#model.Slice.openStart) property as hints, rather
    than fixed start and end points. This method may grow the
    replaced area or close open nodes in the slice in order to get a
    fit that is more in line with WYSIWYG expectations, by dropping
    fully covered parent nodes of the replaced region when they are
    marked [non-defining as
    context](https://prosemirror.net/docs/ref/#model.NodeSpec.definingAsContext), or including an
    open parent node from the slice that _is_ marked as [defining
    its content](https://prosemirror.net/docs/ref/#model.NodeSpec.definingForContent).
    
    This is the method, for example, to handle paste. The similar
    [`replace`](https://prosemirror.net/docs/ref/#transform.Transform.replace) method is a more
    primitive tool which will _not_ move the start and end of its given
    range, and is useful in situations where you need more precise
    control over what happens.
    */
    replaceRange(from, to, slice) {
      replaceRange(this, from, to, slice);
      return this;
    }
    /**
    Replace the given range with a node, but use `from` and `to` as
    hints, rather than precise positions. When from and to are the same
    and are at the start or end of a parent node in which the given
    node doesn't fit, this method may _move_ them out towards a parent
    that does allow the given node to be placed. When the given range
    completely covers a parent node, this method may completely replace
    that parent node.
    */
    replaceRangeWith(from, to, node) {
      replaceRangeWith(this, from, to, node);
      return this;
    }
    /**
    Delete the given range, expanding it to cover fully covered
    parent nodes until a valid replace is found.
    */
    deleteRange(from, to) {
      deleteRange(this, from, to);
      return this;
    }
    /**
    Split the content in the given range off from its parent, if there
    is sibling content before or after it, and move it up the tree to
    the depth specified by `target`. You'll probably want to use
    [`liftTarget`](https://prosemirror.net/docs/ref/#transform.liftTarget) to compute `target`, to make
    sure the lift is valid.
    */
    lift(range, target) {
      lift(this, range, target);
      return this;
    }
    /**
    Join the blocks around the given position. If depth is 2, their
    last and first siblings are also joined, and so on.
    */
    join(pos, depth = 1) {
      join(this, pos, depth);
      return this;
    }
    /**
    Wrap the given [range](https://prosemirror.net/docs/ref/#model.NodeRange) in the given set of wrappers.
    The wrappers are assumed to be valid in this position, and should
    probably be computed with [`findWrapping`](https://prosemirror.net/docs/ref/#transform.findWrapping).
    */
    wrap(range, wrappers) {
      wrap(this, range, wrappers);
      return this;
    }
    /**
    Set the type of all textblocks (partly) between `from` and `to` to
    the given node type with the given attributes.
    */
    setBlockType(from, to = from, type, attrs = null) {
      setBlockType(this, from, to, type, attrs);
      return this;
    }
    /**
    Change the type, attributes, and/or marks of the node at `pos`.
    When `type` isn't given, the existing node type is preserved,
    */
    setNodeMarkup(pos, type, attrs = null, marks) {
      setNodeMarkup(this, pos, type, attrs, marks);
      return this;
    }
    /**
    Set a single attribute on a given node to a new value.
    The `pos` addresses the document content. Use `setDocAttribute`
    to set attributes on the document itself.
    */
    setNodeAttribute(pos, attr, value) {
      this.step(new AttrStep(pos, attr, value));
      return this;
    }
    /**
    Set a single attribute on the document to a new value.
    */
    setDocAttribute(attr, value) {
      this.step(new DocAttrStep(attr, value));
      return this;
    }
    /**
    Add a mark to the node at position `pos`.
    */
    addNodeMark(pos, mark) {
      this.step(new AddNodeMarkStep(pos, mark));
      return this;
    }
    /**
    Remove a mark (or all marks of the given type) from the node at
    position `pos`.
    */
    removeNodeMark(pos, mark) {
      let node = this.doc.nodeAt(pos);
      if (!node)
        throw new RangeError("No node at position " + pos);
      if (mark instanceof Mark) {
        if (mark.isInSet(node.marks))
          this.step(new RemoveNodeMarkStep(pos, mark));
      } else {
        let set = node.marks, found2, steps = [];
        while (found2 = mark.isInSet(set)) {
          steps.push(new RemoveNodeMarkStep(pos, found2));
          set = found2.removeFromSet(set);
        }
        for (let i = steps.length - 1; i >= 0; i--)
          this.step(steps[i]);
      }
      return this;
    }
    /**
    Split the node at the given position, and optionally, if `depth` is
    greater than one, any number of nodes above that. By default, the
    parts split off will inherit the node type of the original node.
    This can be changed by passing an array of types and attributes to
    use after the split (with the outermost nodes coming first).
    */
    split(pos, depth = 1, typesAfter) {
      split(this, pos, depth, typesAfter);
      return this;
    }
    /**
    Add the given mark to the inline content between `from` and `to`.
    */
    addMark(from, to, mark) {
      addMark(this, from, to, mark);
      return this;
    }
    /**
    Remove marks from inline nodes between `from` and `to`. When
    `mark` is a single mark, remove precisely that mark. When it is
    a mark type, remove all marks of that type. When it is null,
    remove all marks of any type.
    */
    removeMark(from, to, mark) {
      removeMark(this, from, to, mark);
      return this;
    }
    /**
    Removes all marks and nodes from the content of the node at
    `pos` that don't match the given new parent node type. Accepts
    an optional starting [content match](https://prosemirror.net/docs/ref/#model.ContentMatch) as
    third argument.
    */
    clearIncompatible(pos, parentType, match) {
      clearIncompatible(this, pos, parentType, match);
      return this;
    }
  };

  // src/schema.js
  function buildSchema(spec, extra = {}) {
    const nodes = {};
    for (const _a of spec.nodes) {
      const _b = _a, { name } = _b, n = __objRest(_b, ["name"]);
      nodes[name] = __spreadValues(__spreadValues({}, n), extra.nodes && extra.nodes[name] || {});
    }
    const marks = {};
    for (const _c of spec.marks) {
      const _d = _c, { name } = _d, m = __objRest(_d, ["name"]);
      marks[name] = __spreadValues(__spreadValues({}, m), extra.marks && extra.marks[name] || {});
    }
    return new Schema({ nodes, marks, topNode: spec.topNode });
  }

  // src/rebase.js
  function applyCommit(doc2, since, steps) {
    const newMap = new Mapping(since.map((s) => s.getMap()));
    const mapping = new Mapping(steps.map((s) => s.getMap())).invert();
    mapping.appendMapping(newMap);
    const tr = new Transform(doc2);
    for (let i = 0, mapFrom = steps.length; i < steps.length; i++) {
      const mapped = steps[i].map(mapping.slice(mapFrom));
      mapFrom--;
      if (mapped && !tr.maybeStep(mapped).failed) {
        mapping.appendMapping(new Mapping(tr.mapping.maps.slice(tr.steps.length - 1)));
        mapping.setMirror(mapFrom, mapping.maps.length - 1);
      }
    }
    return { doc: tr.doc, steps: tr.steps };
  }

  // src/gen.js
  function rng(seed) {
    let a = seed >>> 0;
    return () => {
      a = a + 1831565813 >>> 0;
      let t = a;
      t = Math.imul(t ^ t >>> 15, t | 1);
      t ^= t + Math.imul(t ^ t >>> 7, t | 61);
      return ((t ^ t >>> 14) >>> 0) / 4294967296;
    };
  }
  var words = "the quick brown fox jumps over lazy dog release plan sprint issue page editor server writer step version cut fold table list task review deploy schema".split(" ");
  var people = ["s-ada", "s-grace", "s-linus", "s-sunny-otter", "g-mfrggzdfmztwq2lknnwg", "a-0192f0c4-7b1e-7cc3-9d2a-5f6e7a8b9c0d"];
  var languages = ["", "go", "js", "sql", "text", "bash"];
  var typed = Array.from("abcdefghijklmnopqrstuvwxyz  .,é😀ß");
  var Gen = class {
    constructor(schema2, rnd) {
      this.s = schema2;
      this.n = schema2.nodes;
      this.m = schema2.marks;
      this.rnd = rnd;
    }
    int(n) {
      return Math.floor(this.rnd() * n);
    }
    pick(a) {
      return a[this.int(a.length)];
    }
    sentence(n) {
      return Array.from({ length: n }, () => this.pick(words)).join(" ");
    }
    hex(n) {
      let s = "";
      for (let i = 0; i < n; i++) s += "0123456789abcdef"[this.int(16)];
      return s;
    }
    pageId() {
      return this.hex(8) + "-" + this.hex(4) + "-7" + this.hex(3) + "-" + this.pick(["8", "9", "a", "b"]) + this.hex(3) + "-" + this.hex(12);
    }
    chip() {
      return this.n.issue_chip.create({ key: this.pick(["WEB", "API", "OPS"]) + "-" + (1 + this.int(300)) });
    }
    mention() {
      return this.n.mention.create({ personId: this.pick(people), label: this.pick(["Ada", "Grace Hopper", "Linus", "Sunny Otter"]) });
    }
    pageLink() {
      return this.n.page_link.create({ pageId: this.pageId() });
    }
    link() {
      return this.m.link.create({ href: this.rnd() < 0.8 ? "https://example.com/" + this.pick(words) : "mailto:ada@example.com" });
    }
    inline() {
      const { s, m } = this;
      const parts = [];
      const k = 3 + this.int(6);
      for (let i = 0; i < k; i++) {
        const r = this.rnd();
        if (r < 0.55) parts.push(s.text(this.sentence(3 + this.int(8)) + " "));
        else if (r < 0.65) parts.push(s.text(this.sentence(2) + " ", [m.strong.create()]));
        else if (r < 0.72) parts.push(s.text(this.sentence(2) + " ", [m.em.create()]));
        else if (r < 0.76) parts.push(s.text(this.pick(words), [m.code.create()]), s.text(" "));
        else if (r < 0.82) parts.push(s.text(this.sentence(2), [this.link()]), s.text(" "));
        else if (r < 0.88) parts.push(this.chip(), s.text(" "));
        else if (r < 0.93) parts.push(this.mention(), s.text(" "));
        else if (r < 0.96) parts.push(this.pageLink(), s.text(" "));
        else parts.push(this.n.hard_break.create());
      }
      return parts;
    }
    para() {
      return this.n.paragraph.create(null, this.inline());
    }
    item(t) {
      return t.create(t === this.n.task_item ? { checked: this.rnd() < 0.5 } : null, [this.para()]);
    }
    cell(h) {
      return (h ? this.n.table_header : this.n.table_cell).create(null, [this.n.paragraph.create(null, [this.s.text(this.sentence(2))])]);
    }
    table(rows, cols) {
      return this.n.table.create(null, Array.from({ length: rows }, (_, r) => this.n.table_row.create(null, Array.from({ length: cols }, () => this.cell(r === 0)))));
    }
    code() {
      return this.n.code_block.create({ language: this.pick(languages) }, [this.s.text('func main() {\n	fmt.Println("' + this.sentence(3) + '")\n}')]);
    }
    image() {
      return this.n.image.create({ imageId: "img-" + this.int(40), alt: this.sentence(3) });
    }
    section() {
      const n = this.n;
      const b = [n.heading.create({ level: 1 + this.int(3) }, [this.s.text(this.sentence(4))])];
      b.push(this.para(), this.para());
      b.push(n.bullet_list.create(null, [this.item(n.list_item), this.item(n.list_item), this.item(n.list_item)]));
      b.push(n.task_list.create(null, [this.item(n.task_item), this.item(n.task_item)]));
      if (this.rnd() < 0.5) b.push(n.ordered_list.create({ order: this.rnd() < 0.8 ? 1 : 3 }, [this.item(n.list_item), this.item(n.list_item)]));
      if (this.rnd() < 0.4) b.push(this.code());
      if (this.rnd() < 0.4) b.push(n.blockquote.create(null, [this.para()]));
      if (this.rnd() < 0.3) b.push(this.table(3, 4));
      if (this.rnd() < 0.2) b.push(n.horizontal_rule.create());
      if (this.rnd() < 0.1) b.push(this.image());
      return b;
    }
    // A document of about kb kilobytes of JSON.
    doc(kb) {
      const blocks = [];
      let doc2;
      do {
        blocks.push(...this.section());
        doc2 = this.n.doc.create(null, blocks);
      } while (JSON.stringify(doc2.toJSON()).length < kb * 1024);
      doc2.check();
      return doc2;
    }
    textblocks(d) {
      const out = [];
      d.descendants((node, pos) => {
        if (node.isTextblock) {
          out.push({ node, start: pos + 1, end: pos + 1 + node.content.size });
          return false;
        }
        return true;
      });
      return out;
    }
    // A position inside a surrogate pair moves to the pair's start.
    snap(d, pos) {
      const $p = d.resolve(pos);
      if (!$p.textOffset) return pos;
      const t = $p.parent.child($p.index()).text;
      const c = t.charCodeAt($p.textOffset - 1);
      return c >= 55296 && c <= 56319 ? pos - 1 : pos;
    }
    anyPos(d) {
      return this.snap(d, this.int(d.content.size + 1));
    }
    textPos(d) {
      const t = this.pick(this.textblocks(d));
      return { t, pos: this.snap(d, t.start + this.int(t.end - t.start + 1)) };
    }
    // One edit of the batch fixtures (typing, deletes, marks, splits, joins,
    // block types, attrs, wraps, lifts, pastes, chips). Returns a Transform or null.
    batchEdit(d, noAttr) {
      const { s, n, m } = this;
      const tr = new Transform(d);
      const r = this.rnd();
      const { t, pos } = this.textPos(d);
      const code = t.node.type === n.code_block;
      try {
        if (r < 0.6) tr.replaceWith(pos, pos, s.text(this.pick(typed), d.resolve(pos).marks()));
        else if (r < 0.7) {
          if (pos < t.end) {
            let e = this.snap(d, pos + 1);
            if (e === pos) e = Math.min(pos + 2, t.end);
            tr.delete(pos, e);
          }
        } else if (r < 0.76) {
          if (!code && t.end - t.start > 4) {
            const a = this.snap(d, t.start + this.int(t.end - t.start - 3));
            tr.addMark(a, this.snap(d, a + 3), this.pick([m.strong, m.em, m.strike]).create());
          }
        } else if (r < 0.8) {
          if (!code) tr.removeMark(t.start, t.end, this.pick([m.strong, m.em]));
        } else if (r < 0.85) {
          if (!code && t.node.type === n.paragraph) tr.split(pos);
        } else if (r < 0.89) {
          const $e = d.resolve(t.end + 1);
          if (t.node.type === n.paragraph && $e.nodeAfter && $e.nodeAfter.type === n.paragraph && canJoin(d, t.end + 1)) tr.join(t.end + 1);
        } else if (r < 0.92) {
          const $p = d.resolve(t.start);
          if (t.node.type === n.heading) tr.setNodeMarkup(t.start - 1, null, { level: 1 + this.int(3) });
          else if (t.node.type === n.paragraph && $p.parent.canReplaceWith($p.index(), $p.index() + 1, n.heading)) tr.setBlockType(t.start, t.start, n.heading, { level: 2 });
        } else if (r < 0.95) {
          if (noAttr) return null;
          const items = [];
          d.descendants((node, p) => {
            if (node.type === n.task_item || node.type === n.code_block) items.push({ node, p });
          });
          if (items.length) {
            const it = this.pick(items);
            if (it.node.type === n.task_item) tr.setNodeAttribute(it.p, "checked", !it.node.attrs.checked);
            else tr.setNodeAttribute(it.p, "language", this.pick(languages));
          }
        } else if (r < 0.97) {
          const $f = d.resolve(t.start), range = $f.blockRange();
          if (range && $f.depth === 1 && t.node.type === n.paragraph) tr.wrap(range, [{ type: n.blockquote }]);
          else if (range) {
            const tgt = liftTarget(range);
            if (tgt != null) tr.lift(range, tgt);
          }
        } else if (r < 0.99) {
          if (!code) tr.replaceWith(pos, pos, s.text("pasted " + this.pick(words) + " ", [m.strong.create()]));
        } else {
          if (!code) tr.insert(pos, this.chip());
        }
      } catch (e) {
        return null;
      }
      return tr.steps.length ? tr : null;
    }
    // The steps of one commit of the commit fixtures: five edits (typing,
    // deletes, bold over a textblock) on the version the client sees.
    commitSteps(doc2) {
      const tbs = this.textblocks(doc2).map((t) => [t.start, t.end]);
      const tr = new Transform(doc2);
      for (let i = 0; i < 5; i++) {
        const [a, b] = this.pick(tbs);
        const p = this.snap(tr.doc, tr.mapping.map(a + this.int(b - a + 1)));
        const r = this.rnd();
        if (r < 0.75) tr.replaceWith(p, p, this.s.text(this.pick(["x", "y", "z", "é", "😀"]), tr.doc.resolve(p).marks()));
        else if (r < 0.9 && p < tr.mapping.map(b)) {
          let e = this.snap(tr.doc, p + 1);
          if (e === p) e = p + 2;
          if (e <= tr.mapping.map(b)) tr.delete(p, e);
        } else tr.addMark(tr.mapping.map(a), tr.mapping.map(b), this.m.strong.create());
      }
      return tr.steps;
    }
    // One edit of the fuzz commits: everything an editor does, including
    // deletes across blocks, pastes of open slices, table edits and attrs.
    fuzzEdit(tr) {
      const { s, n, m } = this;
      const d = tr.doc;
      const r = this.int(20);
      const { t, pos } = this.textPos(d);
      const code = t.node.type === n.code_block;
      switch (r) {
        case 0:
        case 1:
        case 2:
        case 3:
          tr.replaceWith(pos, pos, s.text(this.pick(typed), d.resolve(pos).marks()));
          break;
        case 4: {
          if (pos < t.end) tr.delete(pos, this.snap(d, Math.min(t.end, pos + 1 + this.int(6))));
          break;
        }
        case 5: {
          let a = this.anyPos(d), b = this.anyPos(d);
          if (a > b) [a, b] = [b, a];
          if (b - a > 200) b = this.snap(d, a + 200);
          tr.delete(a, b);
          break;
        }
        case 6: {
          const $p = d.resolve(pos);
          const depth = 1 + this.int($p.depth);
          tr.delete($p.before(depth), $p.after(depth));
          break;
        }
        case 7: {
          let a = this.anyPos(d), b = this.anyPos(d);
          if (a > b) [a, b] = [b, a];
          tr.addMark(a, b, this.rnd() < 0.2 ? this.link() : this.pick([m.strong, m.em, m.strike, m.code]).create());
          break;
        }
        case 8: {
          let a = this.anyPos(d), b = this.anyPos(d);
          if (a > b) [a, b] = [b, a];
          tr.removeMark(a, b, this.pick([m.strong, m.em, m.link, null]));
          break;
        }
        case 9:
          if (!code && canSplit(d, pos)) tr.split(pos);
          break;
        case 10: {
          const p = this.anyPos(d);
          if (canJoin(d, p)) tr.join(p);
          break;
        }
        case 11: {
          const $p = d.resolve(t.start);
          const type = this.pick([n.heading, n.paragraph, n.code_block]);
          const attrs = type === n.heading ? { level: 1 + this.int(3) } : type === n.code_block ? { language: this.pick(languages) } : null;
          if ($p.parent.canReplaceWith($p.index(), $p.index() + 1, type)) tr.setBlockType(t.start, t.end, type, attrs);
          break;
        }
        case 12: {
          const items = [];
          d.descendants((node2, p2) => {
            if (!node2.isText && Object.keys(node2.attrs).length) items.push({ node: node2, p: p2 });
          });
          if (!items.length) break;
          const { node, p } = this.pick(items);
          const name = node.type.name;
          if (name === "task_item") tr.setNodeAttribute(p, "checked", !node.attrs.checked);
          else if (name === "heading") tr.setNodeAttribute(p, "level", 1 + this.int(3));
          else if (name === "code_block") tr.setNodeAttribute(p, "language", this.pick(languages));
          else if (name === "ordered_list") tr.setNodeAttribute(p, "order", 1 + this.int(9));
          else if (name === "table_cell" || name === "table_header") tr.setNodeAttribute(p, "colwidth", this.rnd() < 0.5 ? null : [100 + this.int(100)]);
          else if (name === "image") tr.setNodeAttribute(p, "alt", this.sentence(2));
          else if (name === "mention") tr.setNodeAttribute(p, "label", "Ada");
          else if (name === "issue_chip") tr.setNodeAttribute(p, "key", "WEB-" + (1 + this.int(300)));
          break;
        }
        case 13: {
          const $f = d.resolve(t.start), range = $f.blockRange();
          if (!range) break;
          if (this.rnd() < 0.5) {
            const wrapping = findWrapping(range, this.pick([n.blockquote, n.bullet_list, n.task_list, n.ordered_list]));
            if (wrapping) tr.wrap(range, wrapping);
          } else {
            const tgt = liftTarget(range);
            if (tgt != null) tr.lift(range, tgt);
          }
          break;
        }
        case 14: {
          let a = this.anyPos(d), b = this.anyPos(d);
          if (a > b) [a, b] = [b, a];
          if (b - a > 120) b = this.snap(d, a + 120);
          const slice = d.slice(a, b);
          let c = this.anyPos(d), e = this.rnd() < 0.7 ? c : this.anyPos(d);
          if (c > e) [c, e] = [e, c];
          tr.replace(c, e, slice);
          break;
        }
        case 15: {
          if (code) break;
          tr.insert(pos, this.pick([() => this.chip(), () => this.mention(), () => this.pageLink(), () => n.hard_break.create()])());
          break;
        }
        case 16: {
          const $p = d.resolve(pos);
          const at = $p.after(1);
          const node = this.pick([() => n.horizontal_rule.create(), () => this.image(), () => this.table(1 + this.int(2), 1 + this.int(3)), () => this.code(), () => this.para()])();
          tr.insert(at, node);
          break;
        }
        case 17: {
          const rows = [];
          d.descendants((node2, p2) => {
            if (node2.type === n.table_row) rows.push({ node: node2, p: p2 });
            return node2.type !== n.table_row;
          });
          if (!rows.length) break;
          const { node, p } = this.pick(rows);
          if (this.rnd() < 0.6) tr.insert(p + node.nodeSize, node.copy(Fragment.from(node.content.content.map((c) => c.type.createAndFill(c.attrs)))));
          else tr.delete(p, p + node.nodeSize);
          break;
        }
        case 18: {
          let e = this.snap(d, Math.min(t.end, pos + this.int(8)));
          tr.replaceWith(pos, e, s.text(this.pick(words), d.resolve(pos).marks()));
          break;
        }
        case 19: {
          let a = this.anyPos(d), b = this.anyPos(d);
          if (a > b) [a, b] = [b, a];
          if (b - a > 60) b = this.snap(d, a + 60);
          const frag = Fragment.from([n.paragraph.create(null, s.text(this.sentence(2))), n.paragraph.create(null, s.text(this.sentence(1)))]);
          tr.replace(a, b, new Slice(frag, 1, 1));
          break;
        }
      }
    }
    fuzzSteps(doc2, edits) {
      const tr = new Transform(doc2);
      for (let i = 0; i < edits; i++) {
        try {
          this.fuzzEdit(tr);
        } catch (e) {
        }
      }
      return tr.steps;
    }
  };
  function generator(schema2, seed) {
    return new Gen(schema2, rng(seed));
  }
  function fuzzCase(schema2, seed, opts) {
    const g = generator(schema2, seed);
    const kb = opts.kb || 2, commits = opts.commits || 12, maxK = opts.maxK == null ? 6 : opts.maxK, edits = opts.edits || 4;
    let doc2 = g.doc(kb);
    const start = doc2;
    const docs2 = [doc2], history = [];
    const out = [];
    for (let i = 0; i < commits; i++) {
      const head = history.length;
      const base = Math.max(0, head - g.int(maxK + 1));
      const steps = g.fuzzSteps(docs2[base], 1 + g.int(edits));
      let res;
      try {
        res = applyCommit(doc2, history.slice(base).flat(), steps);
      } catch (e) {
        out.push({ base, steps: steps.map((s) => s.toJSON()), error: String(e.message) });
        continue;
      }
      out.push({ base, steps: steps.map((s) => s.toJSON()), applied: res.steps.map((s) => s.toJSON()) });
      history.push(res.steps);
      doc2 = res.doc;
      docs2.push(doc2);
    }
    return { doc: start.toJSON(), commits: out, final: doc2.toJSON() };
  }

  // src/entry.js
  var schema = null;
  var docs = /* @__PURE__ */ new Map();
  var nextId = 1;
  function doc(id) {
    const d = docs.get(id);
    if (!d) throw new RangeError("no document " + id);
    return d;
  }
  function mappingFrom(json) {
    const m = new Mapping();
    for (const [ranges, inverted] of json.maps) m.appendMap(new StepMap(ranges, !!inverted));
    for (let i = 0; i + 1 < (json.mirror || []).length; i += 2) m.setMirror(json.mirror[i], json.mirror[i + 1]);
    return m;
  }
  globalThis.oracle = {
    versions: { model: "1.25.12", transform: "1.12.2" },
    init(specJSON) {
      schema = buildSchema(JSON.parse(specJSON));
      return Object.keys(schema.nodes).length;
    },
    // Node.fromJSON then toJSON: the document as prosemirror-model writes it.
    roundtrip(docJSON) {
      return JSON.stringify(Node.fromJSON(schema, JSON.parse(docJSON)).toJSON());
    },
    // fromJSON and check: "" when valid, else the error message.
    check(docJSON) {
      try {
        Node.fromJSON(schema, JSON.parse(docJSON)).check();
        return "";
      } catch (e) {
        return String(e.message);
      }
    },
    load(docJSON) {
      const id = nextId++;
      docs.set(id, Node.fromJSON(schema, JSON.parse(docJSON)));
      return id;
    },
    free(id) {
      docs.delete(id);
    },
    json(id) {
      return JSON.stringify(doc(id).toJSON());
    },
    // Applies one step as Transform.maybeStep does: "" when it applied,
    // "failed: <reason>" when it failed (the doc stays), and throws otherwise.
    apply(id, stepJSON) {
      const step = Step.fromJSON(schema, JSON.parse(stepJSON));
      const res = step.apply(doc(id));
      if (res.failed) return "failed: " + res.failed;
      docs.set(id, res.doc);
      return "";
    },
    // Rebases a commit's steps over `since` and applies them; returns the
    // steps as applied.
    rebase(id, sinceJSON, stepsJSON) {
      const since = JSON.parse(sinceJSON).map((j) => Step.fromJSON(schema, j));
      const steps = JSON.parse(stepsJSON).map((j) => Step.fromJSON(schema, j));
      const res = applyCommit(doc(id), since, steps);
      docs.set(id, res.doc);
      return JSON.stringify(res.steps.map((s) => s.toJSON()));
    },
    // Maps a step through a mapping given as {maps: [[ranges, inverted]], mirror: []}.
    mapStep(stepJSON, mappingJSON) {
      const mapped = Step.fromJSON(schema, JSON.parse(stepJSON)).map(mappingFrom(JSON.parse(mappingJSON)));
      return mapped ? JSON.stringify(mapped.toJSON()) : "null";
    },
    mapPos(mappingJSON, pos, assoc) {
      const r = mappingFrom(JSON.parse(mappingJSON)).mapResult(pos, assoc);
      return JSON.stringify({ pos: r.pos, deleted: r.deleted, before: r.deletedBefore, after: r.deletedAfter, across: r.deletedAcross });
    },
    stepMap(stepJSON) {
      const m = Step.fromJSON(schema, JSON.parse(stepJSON)).getMap();
      return JSON.stringify({ ranges: m.ranges, inverted: m.inverted });
    },
    diffStart(aJSON, bJSON) {
      const a = Node.fromJSON(schema, JSON.parse(aJSON)), b = Node.fromJSON(schema, JSON.parse(bJSON));
      return JSON.stringify(a.content.findDiffStart(b.content));
    },
    diffEnd(aJSON, bJSON) {
      const a = Node.fromJSON(schema, JSON.parse(aJSON)), b = Node.fromJSON(schema, JSON.parse(bJSON));
      return JSON.stringify(a.content.findDiffEnd(b.content));
    },
    textBetween(docJSON, sep, leaf) {
      const d = Node.fromJSON(schema, JSON.parse(docJSON));
      return d.textBetween(0, d.content.size, sep, leaf);
    },
    // A document of about kb kilobytes made by the fixture generator.
    genDoc(seed, kb) {
      return JSON.stringify(generator(schema, seed).doc(kb).toJSON());
    },
    // A fuzz case for FuzzRebase: {doc, commits: [{base, steps, applied | error}], final}.
    fuzzCase(seed, optsJSON) {
      return JSON.stringify(fuzzCase(schema, seed, JSON.parse(optsJSON)));
    }
  };
})();
