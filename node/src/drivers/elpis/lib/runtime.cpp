#include <stdio.h>
#include <unordered_map>
#include <string>
#include <future>
#include <list>
#include <unistd.h>
#include <mutex>
#include <string.h>
#include <any>
#include <unordered_set>
#include <iostream>
#include <vector>
#include <queue>
#include <atomic>
#include <wasmedge/wasmedge.h>
#include <sstream>
#include <map>

#include "../../lib/nlohmann/json.hpp"
using json = nlohmann::json;

using namespace std;

function<char *(char *)> send;

const uint32_t i16_t = 1;
const uint32_t i32_t = 0xf1;
const uint32_t i64_t = 0xf2;
const uint32_t f32_t = 4;
const uint32_t f64_t = 5;
const uint32_t bool_t = 0xf3;
const uint32_t str_t = 0xf4;
const uint32_t arr_t = 0xf7;
const uint32_t obj_t = 0xf6;
const uint32_t und_t = 0xf5;
const uint32_t iden_t = 0xf0;
const uint32_t fn_t = 0xfe;

typedef unordered_map<std::string, pair<uint32_t, any> *> MemLayer;

class Memory
{
public:
  vector<MemLayer> data{};
  vector<pair<size_t, size_t>> retPoints{};
  vector<pair<uint32_t, any> *> retDests{};
  void add(string varName, pair<uint32_t, any> *data)
  {
    this->data[this->data.size() - 1].insert({varName, data});
  }
  void set(string varName, pair<uint32_t, any> *data)
  {
    for (vector<MemLayer>::reverse_iterator layer = this->data.rbegin(); layer != this->data.rend(); layer++)
    {
      if (layer->find(varName) != layer->end())
      {
        layer->insert({varName, data});
      }
    }
  }
  pair<uint32_t, any> *get(string varName)
  {
    for (vector<MemLayer>::reverse_iterator layer = this->data.rbegin(); layer != this->data.rend(); layer++)
    {
      if (auto unit = layer->find(varName); unit != layer->end())
      {
        return unit->second;
      }
    }
    return new pair<uint32_t, any>(und_t, NULL);
  }
  void setAsReturnVal(pair<uint32_t, any> *data)
  {
    this->retDests[this->retDests.size() - 1]->first = data->first;
    this->retDests[this->retDests.size() - 1]->second = data->second;
  }
  void pushLayer(size_t retPoint, size_t retEndPoint, pair<uint32_t, any> *res)
  {
    this->retPoints.push_back({retPoint, retEndPoint});
    this->retDests.push_back(res);
    this->data.push_back({});
  }
  void pushLayer(size_t retPoint, size_t retEndPoint)
  {
    this->pushLayer(retPoint, retEndPoint, new pair<uint32_t, any>(und_t, NULL));
  }
  pair<size_t, size_t> popLayer()
  {
    this->data.pop_back();
    this->retDests.pop_back();
    auto retPoint = this->retPoints[this->retPoints.size() - 1];
    this->retPoints.pop_back();
    return retPoint;
  }
};

class Type
{
public:
  uint32_t typenum = 0;
  unordered_set<uint32_t> parents{};
  virtual pair<int, any> parse(Memory &mem, unordered_map<uint32_t, Type *> &types, vector<unsigned char>, int start) = 0;
  Type(uint32_t t)
  {
    this->typenum = t;
  }
};

class Id
{
public:
  std::string name;
  Id(std::string n)
  {
    this->name = n;
  }
};

class BinaryExp
{
public:
  pair<uint32_t, any> operand1;
  pair<uint32_t, any> operand2;
  char command;
  BinaryExp(char command, pair<uint32_t, any> op1, pair<uint32_t, any> op2)
  {
    this->command = command;
    this->operand1 = op1;
    this->operand2 = op2;
  }
};

class MemberExp
{
public:
  static const unsigned char OPCODE = 0x05;
  pair<uint32_t, any> object;
  pair<uint32_t, any> property;
  MemberExp(pair<uint32_t, any> obj, pair<uint32_t, any> prop)
  {
    this->object = obj;
    this->property = prop;
  }
};

class Call
{
public:
  pair<uint32_t, any> callee;
  vector<pair<uint32_t, any>> args;
  Call(pair<uint32_t, any> callee, vector<pair<uint32_t, any>> args)
  {
    this->callee = callee;
    this->args = args;
  }
};

class Fn
{
public:
  std::string id;
  vector<pair<uint32_t, any>> params;
  uint32_t startPos;
  uint32_t length;
  Fn() {}
  Fn(
      std::string id,
      vector<pair<uint32_t, any>> params,
      uint32_t startPos,
      uint32_t length)
  {
    this->id = id;
    this->params = params;
    this->startPos = startPos;
    this->length = length;
  }
};

class Utils
{
public:
  static std::string toString(pair<uint32_t, any> *val)
  {
    if (val->first == i16_t)
    {
      return to_string(any_cast<short>(val->second));
    }
    else if (val->first == i32_t)
    {
      return to_string(any_cast<int>(val->second));
    }
    else if (val->first == i64_t)
    {
      return to_string(any_cast<long>(val->second));
    }
    else if (val->first == f32_t)
    {
      return to_string(any_cast<float>(val->second));
    }
    else if (val->first == f64_t)
    {
      return to_string(any_cast<double>(val->second));
    }
    else if (val->first == bool_t)
    {
      return any_cast<bool>(val->second) ? "true" : "false";
    }
    else if (val->first == str_t)
    {
      return any_cast<std::string>(val->second);
    }
    else
    {
      return "";
    }
  }
  static any modifyNumber(uint32_t t, any val, int c)
  {
    switch (t)
    {
    case i16_t:
    {
      return any_cast<short>(val) + c;
    }
    case i32_t:
    {
      return any_cast<int>(val) + c;
    }
    case i64_t:
    {
      return any_cast<long>(val) + c;
    }
    case f32_t:
    {
      return any_cast<float>(val) + c;
    }
    case f64_t:
    {
      return any_cast<double>(val) + c;
    }
    default:
    {
      return 0;
    }
    }
  }
  static int parseDataAsInt(vector<unsigned char> buffer)
  {
    return uint32_t((unsigned char)(buffer[0]) << 24 |
                    (unsigned char)(buffer[1]) << 16 |
                    (unsigned char)(buffer[2]) << 8 |
                    (unsigned char)(buffer[3]));
  }
  static vector<unsigned char> pickSubarray(vector<unsigned char> A, int i, int j)
  {
    vector<unsigned char> sub = vector<unsigned char>(j, 0);
    for (int x = i; x < i + j; x++)
    {
      sub[x - i] = A[x];
    }
    return sub;
  }
  static bool startswith(const std::string &str, const std::string &cmp)
  {
    return str.compare(0, cmp.length(), cmp) == 0;
  }
  static std::string pickString(vector<unsigned char> A, int i, int j)
  {
    vector<unsigned char> da = Utils::pickSubarray(A, i, j);
    std::string str((const char *)da.data(), da.size());
    return str;
  }
};

static pair<pair<uint32_t, any>, int> pickVal(Memory &mem, unordered_map<uint32_t, Type *> &types, vector<unsigned char> A, int pc)
{
  int pos = pc;
  uint32_t valType = A[pos];
  pos += 1;
  if (valType == iden_t)
  {
    auto nameLength = Utils::parseDataAsInt(Utils::pickSubarray(A, pos, 4));
    pos += 4;
    auto name = Utils::pickString(A, pos, nameLength);
    pos += nameLength;
    auto id = Id(name);
    return {{iden_t, id}, pos};
  }
  else if (valType == 0xf8)
  {
    auto command = A[pos];
    pos++;
    auto operand1 = pickVal(mem, types, A, pos);
    pos = operand1.second;
    auto operand2 = pickVal(mem, types, A, pos);
    pos = operand2.second;
    auto o = BinaryExp(command, operand1.first, operand2.first);
    return {{valType, o}, pos};
  }
  else if (valType == 0xf9)
  {
    auto obj = pickVal(mem, types, A, pos);
    pos = obj.second;
    auto prop = pickVal(mem, types, A, pos);
    pos = prop.second;
    auto mexp = MemberExp(obj.first, prop.first);
    return {{valType, mexp}, pos};
  }
  else if (valType == 0xfc)
  {
    auto callee = pickVal(mem, types, A, pos);
    pos = callee.second;
    auto argsCount = Utils::parseDataAsInt(Utils::pickSubarray(A, pos, 4));
    pos += 4;
    vector<pair<uint32_t, any>> arr{};
    for (int i = 0; i < argsCount; i++)
    {
      auto parsedItem = pickVal(mem, types, A, pos);
      arr.push_back({parsedItem.first.first, parsedItem.first.second});
      pos = parsedItem.second;
    }
    auto cexp = Call(callee.first, arr);
    return {{valType, cexp}, pos};
  }
  else if (valType == fn_t)
  {
    auto paramsCount = Utils::parseDataAsInt(Utils::pickSubarray(A, pos, 4));
    pos += 4;
    vector<pair<uint32_t, any>> arr{};
    for (int i = 0; i < paramsCount; i++)
    {
      auto parsedItem = pickVal(mem, types, A, pos);
      arr.push_back({parsedItem.first.first, parsedItem.first.second});
      pos = parsedItem.second;
    }
    uint32_t length = Utils::parseDataAsInt(Utils::pickSubarray(A, pos, 4));
    pos += 4;
    uint32_t startPos = pos;
    pos += length;
    auto fn = Fn("", arr, startPos, length);
    return {{valType, fn}, pos};
  }
  else
  {
    auto typ = types[valType];
    auto parsed = typ->parse(mem, types, A, pos);
    pos += parsed.first;
    return {{valType, parsed.second}, pos};
  }
}

class Array
{
public:
  vector<pair<uint32_t, any> *> items{};
  Array(vector<pair<uint32_t, any> *> its)
  {
    this->items = its;
  }
};

class Object
{
public:
  map<string, pair<uint32_t, any> *> props{};
  Object(map<string, pair<uint32_t, any> *> ps)
  {
    this->props = ps;
  }
  pair<uint32_t, any> *getProp(string propKey)
  {
    return this->props[propKey];
  }
};

class Ref
{
public:
  void *pointer;
  char targetType;
  Ref(char tt, void *p)
  {
    this->targetType = tt;
    this->pointer = p;
  }
};

class TypeI16 : public Type
{
public:
  TypeI16() : Type(i16_t) {}
  pair<int, any> parse(Memory &mem, unordered_map<uint32_t, Type *> &types, vector<unsigned char> buffer, int start)
  {
    return {2, short((unsigned char)(buffer[start + 0]) << 8 |
                     (unsigned char)(buffer[start + 1]))};
  }
};

class TypeI32 : public Type
{
public:
  TypeI32() : Type(i32_t) {}
  pair<int, any> parse(Memory &mem, unordered_map<uint32_t, Type *> &types, vector<unsigned char> buffer, int start)
  {
    return {4, int((unsigned char)(buffer[start + 0]) << 24 |
                   (unsigned char)(buffer[start + 1]) << 16 |
                   (unsigned char)(buffer[start + 2]) << 8 |
                   (unsigned char)(buffer[start + 3]))};
  }
};

class TypeI64 : public Type
{
public:
  TypeI64() : Type(i64_t) {}
  pair<int, any> parse(Memory &mem, unordered_map<uint32_t, Type *> &types, vector<unsigned char> buffer, int start)
  {
    return {8, long((unsigned char)(buffer[start + 0]) << 56 |
                    (unsigned char)(buffer[start + 1]) << 48 |
                    (unsigned char)(buffer[start + 2]) << 40 |
                    (unsigned char)(buffer[start + 3]) << 32 |
                    (unsigned char)(buffer[start + 4]) << 24 |
                    (unsigned char)(buffer[start + 5]) << 16 |
                    (unsigned char)(buffer[start + 6]) << 8 |
                    (unsigned char)(buffer[start + 7]))};
  }
};

class TypeF32 : public Type
{
public:
  TypeF32() : Type(f32_t) {}
  pair<int, any> parse(Memory &mem, unordered_map<uint32_t, Type *> &types, vector<unsigned char> buffer, int start)
  {
    return {4, float((unsigned char)(buffer[start + 0]) << 24 |
                     (unsigned char)(buffer[start + 1]) << 16 |
                     (unsigned char)(buffer[start + 2]) << 8 |
                     (unsigned char)(buffer[start + 3]))};
  }
};

class TypeF64 : public Type
{
public:
  TypeF64() : Type(f64_t) {}
  pair<int, any> parse(Memory &mem, unordered_map<uint32_t, Type *> &types, vector<unsigned char> buffer, int start)
  {
    return {8, double((unsigned char)(buffer[start + 0]) << 56 |
                      (unsigned char)(buffer[start + 1]) << 48 |
                      (unsigned char)(buffer[start + 2]) << 40 |
                      (unsigned char)(buffer[start + 3]) << 32 |
                      (unsigned char)(buffer[start + 4]) << 24 |
                      (unsigned char)(buffer[start + 5]) << 16 |
                      (unsigned char)(buffer[start + 6]) << 8 |
                      (unsigned char)(buffer[start + 7]))};
  }
};

class TypeBool : public Type
{
public:
  TypeBool() : Type(bool_t) {}
  pair<int, any> parse(Memory &mem, unordered_map<uint32_t, Type *> &types, vector<unsigned char> buffer, int start)
  {
    return {1, buffer[start + 0] == 0x01};
  }
};

class TypeStr : public Type
{
public:
  TypeStr() : Type(str_t) {}
  pair<int, any> parse(Memory &mem, unordered_map<uint32_t, Type *> &types, vector<unsigned char> buffer, int start)
  {
    int strLength = int((unsigned char)(buffer[start + 0]) << 24 |
                        (unsigned char)(buffer[start + 1]) << 16 |
                        (unsigned char)(buffer[start + 2]) << 8 |
                        (unsigned char)(buffer[start + 3]));
    vector<unsigned char> v(buffer.begin() + start + 4, buffer.begin() + start + 4 + strLength);
    std::string str((const char *)v.data(), v.size());
    return {4 + strLength, str};
  }
};

class TypeObj : public Type
{
public:
  TypeObj() : Type(obj_t) {}
  pair<int, any> parse(Memory &mem, unordered_map<uint32_t, Type *> &types, vector<unsigned char> buffer, int start)
  {
    int propsCount = int((unsigned char)(buffer[start + 0]) << 24 |
                         (unsigned char)(buffer[start + 1]) << 16 |
                         (unsigned char)(buffer[start + 2]) << 8 |
                         (unsigned char)(buffer[start + 3]));

    int pos = start + 4;
    map<string, pair<uint32_t, any> *> obj{};
    for (int i = 0; i < propsCount; i++)
    {
      int strLength = int((unsigned char)(buffer[pos + 0]) << 24 |
                          (unsigned char)(buffer[pos + 1]) << 16 |
                          (unsigned char)(buffer[pos + 2]) << 8 |
                          (unsigned char)(buffer[pos + 3]));
      pos += 4;
      vector<unsigned char> v(buffer.begin() + pos, buffer.begin() + pos + strLength);
      std::string key((const char *)v.data(), v.size());
      pos += strLength;
      auto parsedItem = pickVal(mem, types, buffer, pos);
      obj[key] = new pair<uint32_t, any>(parsedItem.first.first, parsedItem.first.second);
      pos = parsedItem.second;
    }
    return {pos - start, Ref(1, new Object(obj))};
  }
};

class TypeArr : public Type
{
public:
  TypeArr() : Type(arr_t) {}
  pair<int, any> parse(Memory &mem, unordered_map<uint32_t, Type *> &types, vector<unsigned char> buffer, int start)
  {
    int itemsCount = int((unsigned char)(buffer[start + 0]) << 24 |
                         (unsigned char)(buffer[start + 1]) << 16 |
                         (unsigned char)(buffer[start + 2]) << 8 |
                         (unsigned char)(buffer[start + 3]));

    int pos = start + 4;
    vector<pair<uint32_t, any> *> arr{};
    for (int i = 0; i < itemsCount; i++)
    {
      auto parsedItem = pickVal(mem, types, buffer, pos);
      arr.push_back(new pair<uint32_t, any>(parsedItem.first.first, parsedItem.first.second));
      pos = parsedItem.second;
    }
    return {pos - start, Ref(2, new Array(arr))};
  }
};

class TypeUnd : public Type
{
public:
  TypeUnd() : Type(arr_t) {}
  pair<int, any> parse(Memory &mem, unordered_map<uint32_t, Type *> &types, vector<unsigned char> buffer, int start)
  {
    return {1, NULL};
  }
};

class Operation
{
public:
  unsigned char opcode;
  Operation(unsigned char oc)
  {
    this->opcode = oc;
  }
};

class DefineVar : public Operation
{
public:
  static const unsigned char OPCODE = 0x01;
  std::string name;
  pair<uint32_t, any> *value;
  DefineVar(std::string name, uint32_t typenum, any val) : Operation(OPCODE)
  {
    this->name = name;
    this->value = new pair<uint32_t, any>(typenum, val);
  }
};

class VarPlusPlus : public Operation
{
public:
  static const unsigned char OPCODE = 0x66;
  pair<uint32_t, any> name;
  VarPlusPlus(pair<uint32_t, any> name) : Operation(OPCODE)
  {
    this->name = name;
  }
};

class VarMinMin : public Operation
{
public:
  static const unsigned char OPCODE = 0x67;
  pair<uint32_t, any> name;
  VarMinMin(pair<uint32_t, any> name) : Operation(OPCODE)
  {
    this->name = name;
  }
};

class ExpOp : public Operation
{
public:
  static const unsigned char OPCODE = 0x02;
  pair<uint32_t, any> exp;
  ExpOp(pair<uint32_t, any> exp) : Operation(OPCODE)
  {
    this->exp = exp;
  }
};

class DefineFunc : public Operation
{
public:
  static const unsigned char OPCODE = 0x03;
  Fn func;
  DefineFunc(Fn fn) : Operation(OPCODE)
  {
    this->func = fn;
  }
};

class ReturnOp : public Operation
{
public:
  static const unsigned char OPCODE = 0x05;
  pair<uint32_t, any> arg;
  ReturnOp(pair<uint32_t, any> arg) : Operation(OPCODE)
  {
    this->arg = arg;
  }
};

class LockData : public Operation
{
public:
  static const unsigned char OPCODE = 0x69;
  std::string name;
  vector<std::string> keys;
  LockData(std::string name, vector<std::string> keys) : Operation(OPCODE)
  {
    this->name = name;
    this->keys = keys;
  }
};

class UnlockData : public Operation
{
public:
  static const unsigned char OPCODE = 0x6a;
  std::string name;
  vector<std::string> keys;
  UnlockData(std::string name, vector<std::string> keys) : Operation(OPCODE)
  {
    this->name = name;
    this->keys = keys;
  }
};

atomic<int> doneTasks = 0;

std::mutex globalLock;

const int COUNT = 5;

bool isPaused(int index);
void notifyInstCount(int index, uint64_t instNum);
void check(int index);
void doCritical();
void runTask(function<void(void *)>, int index);

extern void endProgram();

WasmEdge_Result WasmLock(void *data, const WasmEdge_CallingFrameContext *, const WasmEdge_Value *In, WasmEdge_Value *Out);
WasmEdge_Result WasmUnlock(void *data, const WasmEdge_CallingFrameContext *, const WasmEdge_Value *In, WasmEdge_Value *Out);

class ThreadPool
{
public:
  ThreadPool(size_t num_threads = thread::hardware_concurrency())
  {
    for (size_t i = 0; i < num_threads; ++i)
    {
      threads_.emplace_back([this]
                            {
                  while (true) {
                      function<void()> task;
                      {
                          unique_lock<mutex> lock(
                              queue_mutex_);
                          cv_.wait(lock, [this] {
                              return !tasks_.empty() || stop_;
                          });
                          if (stop_ && tasks_.empty()) {
                              return;
                          }
                          task = move(tasks_.front());
                          tasks_.pop();
                      }
  
                      task();
                  } });
    }
  }
  ~ThreadPool()
  {
    {
      unique_lock<mutex> lock(queue_mutex_);
      stop_ = true;
    }

    cv_.notify_all();

    for (auto &thread : threads_)
    {
      thread.join();
    }
  }

  void enqueue(function<void()> task)
  {
    {
      unique_lock<std::mutex> lock(queue_mutex_);
      tasks_.emplace(move(task));
    }
    cv_.notify_one();
  }

private:
  vector<thread> threads_;
  queue<function<void()>> tasks_;
  mutex queue_mutex_;
  condition_variable cv_;
  bool stop_ = false;
};

class DbOp
{
  std::string type;
  std::string data;
};

class DataUnit
{
public:
  std::string tableId;
  std::string id;
  map<std::string, pair<uint32_t, any>> data{};
  DataUnit(std::string tableId, map<std::string, pair<uint32_t, any>> templ)
  {
    this->tableId = tableId;
    this->id = any_cast<std::string>(templ["id"].second);
    for (auto prop : templ)
    {
      this->data[prop.first] = {prop.second.first, prop.second.second};
    }
  }
  pair<uint32_t, any> getProp(std::string key)
  {
    return this->data[key];
  }
  void setProp(std::string key, any value)
  {
    if (this->data.find(key) != this->data.end())
    {
      this->data[key].second = value;
    }
  }
};

class DataGroup
{
  std::string tableId;
  function<void(std::string, std::string)> removeSingle;
  function<void(DataUnit *)> insertSingle;
  function<DataUnit *(std::string, std::string)> finderSingle;
  function<vector<DataUnit *>(std::string, vector<std::string>)> finderBatch;
  unordered_map<std::string, DataUnit *> dict{};

public:
  DataGroup(
      std::string tableId,
      function<void(std::string, std::string)> rmv,
      function<void(DataUnit *)> isrt,
      function<DataUnit *(std::string, std::string)> fndrSingle,
      function<vector<DataUnit *>(std::string, vector<std::string>)> fndrBatch)
  {
    this->tableId = tableId;
    this->removeSingle = rmv;
    this->insertSingle = isrt;
    this->finderSingle = fndrSingle;
    this->finderBatch = fndrBatch;
  }
  void remove(std::string id)
  {
    this->removeSingle(this->tableId, id);
    this->dict.erase(id);
  }
  void insert(DataUnit *item)
  {
    this->insertSingle(item);
    this->dict[item->id] = item;
  }
  DataUnit *find(std::string id)
  {
    DataUnit *item = this->finderSingle(this->tableId, id);
    item->setProp("id", id);
    this->dict[id] = item;
    return item;
  }
  vector<DataUnit *> find(vector<std::string> ids)
  {
    vector<DataUnit *> res = this->finderBatch(this->tableId, ids);
    for (auto item : res)
    {
      this->dict[item->id] = item;
    }
    return res;
  }
};

class DataTable
{
public:
  std::string name;
  static map<std::string, DataTable *> tables;
  map<std::string, pair<uint32_t, any>> props{};
  DataTable(std::string nm, map<std::string, pair<uint32_t, any>> ps)
  {
    this->name = nm;
    this->props = ps;
    DataTable::tables[nm] = this;
  }
  DataUnit NewDataUnit(std::string tableId)
  {
    return DataUnit(tableId, DataTable::tables[tableId]->props);
  }
};

map<std::string, DataTable *> DataTable::tables = {};

class Trx
{
public:
  vector<DbOp> ops{};
  map<std::string, DataUnit *> store{};
  map<std::string, DataTable *> tables{};
  map<std::string, DataGroup *> data{};
  Trx()
  {
    this->createTable("users");
  }
  void createTable(std::string tableId)
  {
    this->tables[tableId] = new DataTable(tableId, {{"name", {str_t, "anon"}},
                                                    {"age", {i32_t, 27}}});
    this->data[tableId] = new DataGroup(tableId, [this](std::string tblId, std::string id)
                                        { this->remove(tblId, id); }, [this](DataUnit *item)
                                        { this->insert(item); }, [this](std::string tblId, std::string id)
                                        { return this->findSingle(tblId, id); }, [this](std::string tblId, vector<std::string> ids)
                                        { return this->findBatch(tblId, ids); });
  }
  void remove(std::string tableId, std::string id)
  {
    this->store.erase(id);
  }
  void insert(DataUnit *item)
  {
    // TODO: load data from database
    this->store[item->id] = item;
  }
  DataUnit *findSingle(std::string tableId, std::string id)
  {
    // TODO: load data from database
    DataUnit *item = new DataUnit(tableId, {});
    item->setProp("id", id);
    this->store[id] = item;
    return item;
  }
  vector<DataUnit *> findBatch(std::string tableId, vector<std::string> ids)
  {
    vector<DataUnit *> res = vector<DataUnit *>(ids.size(), NULL);
    // TODO: load data from database
    int i = 0;
    for (auto id : ids)
    {
      DataUnit *item = new DataUnit(tableId, {});
      item->setProp("id", id);
      this->store[id] = item;
      res[i] = item;
      i++;
    }
    return res;
  }
};

uint32_t checkTypeOfAny(any x)
{
  uint32_t res = und_t;
  if (x.type() == typeid(short))
  {
    res = i16_t;
  }
  else if (x.type() == typeid(int))
  {
    res = i32_t;
  }
  else if (x.type() == typeid(long))
  {
    res = i64_t;
  }
  else if (x.type() == typeid(float))
  {
    res = f32_t;
  }
  else if (x.type() == typeid(double))
  {
    res = f64_t;
  }
  else if (x.type() == typeid(bool))
  {
    res = bool_t;
  }
  else if (x.type() == typeid(std::string))
  {
    res = str_t;
  }
  return res;
}

class Runtime
{
public:
  bool onchain;
  function<char *(char *)> callback;
  char *id;
  int index;
  uint64_t instCounter{1};
  unordered_map<uint32_t, Type *> types{};
  Memory mem{};
  vector<unsigned char> rawCode;
  vector<uint32_t> jumps;
  vector<pair<unsigned char, Operation *> *> code;
  std::priority_queue<uint64_t, std::vector<uint64_t>, std::greater<uint64_t>> triggerQueue{};
  unordered_map<uint64_t, unordered_map<char *, uint32_t>> triggerListeners{};
  bool newTiggersPendingToAdd{false};
  bool paused{false};
  uint64_t pc = 0;
  uint64_t end = 0;
  void *result;
  mutex tirggerLock;
  thread looper;
  queue<function<void()>> tasks{};
  mutex queue_mutex_;
  condition_variable cv_;
  WasmEdge_VMContext *vm;
  bool stop_ = false;
  atomic<uint64_t> triggerFront = 0;
  bool (*areAllPassed)(uint64_t instCounter, std::unordered_map<char *, uint32_t> waiters, char *myId);
  void (*tryToCheckTrigger)(char *vmId, uint32_t resNum, uint64_t instNum, char *myId);
  int (*lock)(char *vmId, uint32_t resNum, bool shouldLock);
  void (*unlock)(char *vmId, uint32_t resNum);
  vector<tuple<vector<std::string>, uint64_t, uint64_t, std::string>> syncTasks{};
  atomic<int> step = 0;
  bool recording = false;
  void loadDefaultTypes()
  {
    this->types[i16_t] = new TypeI16();
    this->types[i32_t] = new TypeI32();
    this->types[i64_t] = new TypeI64();
    this->types[f32_t] = new TypeF32();
    this->types[f64_t] = new TypeF64();
    this->types[bool_t] = new TypeBool();
    this->types[str_t] = new TypeStr();
    this->types[obj_t] = new TypeObj();
    this->types[arr_t] = new TypeArr();
    this->types[und_t] = new TypeUnd();
  }
  pair<uint32_t, any> *createEmptyUnit()
  {
    return new pair<uint32_t, any>(und_t, NULL);
  }
  void resolveValue(pair<uint32_t, any> exp, bool createIfNotExist, pair<uint32_t, any> *res)
  {
    if (exp.first == iden_t)
    {
      auto IdName = any_cast<Id>(exp.second).name;
      if (auto it = this->mem.get(IdName); it->first != und_t)
      {
        res->first = it->first;
        res->second = it->second;
      }
      else
      {
        *res = {und_t, NULL};
        if (createIfNotExist)
        {
          this->mem.add(IdName, res);
        }
      }
    }
    else if (exp.first == 0xf8)
    {
      BinaryExp bo = any_cast<BinaryExp>(exp.second);
      pair<uint32_t, any> *op1 = createEmptyUnit();
      pair<uint32_t, any> *op2 = createEmptyUnit();
      resolveValue(bo.operand1, false, op1);
      resolveValue(bo.operand2, false, op2);
      if (bo.command == 0x01)
      {
        if (op1->first == i16_t)
        {
          short operand1 = any_cast<short>(op1->second);
          if (op2->first == i16_t)
          {
            short operand2 = any_cast<short>(op2->second);
            res->second = operand1 + operand2;
          }
          else if (op2->first == i32_t)
          {
            int operand2 = any_cast<int>(op2->second);
            res->second = operand1 + operand2;
          }
          else if (op2->first == i64_t)
          {
            long operand2 = any_cast<long>(op2->second);
            res->second = operand1 + operand2;
          }
          else if (op2->first == f32_t)
          {
            float operand2 = any_cast<float>(op2->second);
            res->second = operand1 + operand2;
          }
          else if (op2->first == f64_t)
          {
            double operand2 = any_cast<double>(op2->second);
            res->second = operand1 + operand2;
          }
          else if (op2->first == str_t)
          {
            string operand2 = any_cast<string>(op2->second);
            res->second = to_string(operand1) + operand2;
          }
          else if (op2->first == bool_t)
          {
            bool operand2 = any_cast<bool>(op2->second);
            res->second = operand1 + (operand2 ? 1 : 0);
          }
        }
        else if (op1->first == i32_t)
        {
          int operand1 = any_cast<int>(op1->second);
          if (op2->first == i16_t)
          {
            short operand2 = any_cast<short>(op2->second);
            res->second = operand1 + operand2;
          }
          else if (op2->first == i32_t)
          {
            int operand2 = any_cast<int>(op2->second);
            res->second = operand1 + operand2;
          }
          else if (op2->first == i64_t)
          {
            long operand2 = any_cast<long>(op2->second);
            res->second = operand1 + operand2;
          }
          else if (op2->first == f32_t)
          {
            float operand2 = any_cast<float>(op2->second);
            res->second = operand1 + operand2;
          }
          else if (op2->first == f64_t)
          {
            double operand2 = any_cast<double>(op2->second);
            res->second = operand1 + operand2;
          }
          else if (op2->first == str_t)
          {
            string operand2 = any_cast<string>(op2->second);
            res->second = to_string(operand1) + operand2;
          }
          else if (op2->first == bool_t)
          {
            bool operand2 = any_cast<bool>(op2->second);
            res->second = operand1 + (operand2 ? 1 : 0);
          }
        }
        else if (op1->first == i64_t)
        {
          long operand1 = any_cast<long>(op1->second);
          if (op2->first == i16_t)
          {
            short operand2 = any_cast<short>(op2->second);
            res->second = operand1 + operand2;
          }
          else if (op2->first == i32_t)
          {
            int operand2 = any_cast<int>(op2->second);
            res->second = operand1 + operand2;
          }
          else if (op2->first == i64_t)
          {
            long operand2 = any_cast<long>(op2->second);
            res->second = operand1 + operand2;
          }
          else if (op2->first == f32_t)
          {
            float operand2 = any_cast<float>(op2->second);
            res->second = operand1 + operand2;
          }
          else if (op2->first == f64_t)
          {
            double operand2 = any_cast<double>(op2->second);
            res->second = operand1 + operand2;
          }
          else if (op2->first == str_t)
          {
            string operand2 = any_cast<string>(op2->second);
            res->second = to_string(operand1) + operand2;
          }
          else if (op2->first == bool_t)
          {
            bool operand2 = any_cast<bool>(op2->second);
            res->second = operand1 + (operand2 ? 1 : 0);
          }
        }
        else if (op1->first == f32_t)
        {
          float operand1 = any_cast<float>(op1->second);
          if (op2->first == i16_t)
          {
            short operand2 = any_cast<short>(op2->second);
            res->second = operand1 + operand2;
          }
          else if (op2->first == i32_t)
          {
            int operand2 = any_cast<int>(op2->second);
            res->second = operand1 + operand2;
          }
          else if (op2->first == i64_t)
          {
            long operand2 = any_cast<long>(op2->second);
            res->second = operand1 + operand2;
          }
          else if (op2->first == f32_t)
          {
            float operand2 = any_cast<float>(op2->second);
            res->second = operand1 + operand2;
          }
          else if (op2->first == f64_t)
          {
            double operand2 = any_cast<double>(op2->second);
            res->second = operand1 + operand2;
          }
          else if (op2->first == str_t)
          {
            string operand2 = any_cast<string>(op2->second);
            res->second = to_string(operand1) + operand2;
          }
          else if (op2->first == bool_t)
          {
            bool operand2 = any_cast<bool>(op2->second);
            res->second = operand1 + (operand2 ? 1 : 0);
          }
        }
        else if (op1->first == f64_t)
        {
          double operand1 = any_cast<double>(op1->second);
          if (op2->first == i16_t)
          {
            short operand2 = any_cast<short>(op2->second);
            res->second = operand1 + operand2;
          }
          else if (op2->first == i32_t)
          {
            int operand2 = any_cast<int>(op2->second);
            res->second = operand1 + operand2;
          }
          else if (op2->first == i64_t)
          {
            long operand2 = any_cast<long>(op2->second);
            res->second = operand1 + operand2;
          }
          else if (op2->first == f32_t)
          {
            float operand2 = any_cast<float>(op2->second);
            res->second = operand1 + operand2;
          }
          else if (op2->first == f64_t)
          {
            double operand2 = any_cast<double>(op2->second);
            res->second = operand1 + operand2;
          }
          else if (op2->first == str_t)
          {
            string operand2 = any_cast<string>(op2->second);
            res->second = to_string(operand1) + operand2;
          }
          else if (op2->first == bool_t)
          {
            bool operand2 = any_cast<bool>(op2->second);
            res->second = operand1 + (operand2 ? 1 : 0);
          }
        }
        else if (op1->first == str_t)
        {
          string operand1 = any_cast<string>(op1->second);
          if (op2->first == i16_t)
          {
            short operand2 = any_cast<short>(op2->second);
            res->second = operand1 + to_string(operand2);
          }
          else if (op2->first == i32_t)
          {
            int operand2 = any_cast<int>(op2->second);
            res->second = operand1 + to_string(operand2);
          }
          else if (op2->first == i64_t)
          {
            long operand2 = any_cast<long>(op2->second);
            res->second = operand1 + to_string(operand2);
          }
          else if (op2->first == f32_t)
          {
            float operand2 = any_cast<float>(op2->second);
            res->second = operand1 + to_string(operand2);
          }
          else if (op2->first == f64_t)
          {
            double operand2 = any_cast<double>(op2->second);
            res->second = operand1 + to_string(operand2);
          }
          else if (op2->first == str_t)
          {
            string operand2 = any_cast<string>(op2->second);
            res->second = operand1 + operand2;
          }
          else if (op2->first == bool_t)
          {
            bool operand2 = any_cast<bool>(op2->second);
            res->second = operand1 + (operand2 ? "true" : "false");
          }
        }
        else if (op1->first == bool_t)
        {
          bool operand1 = any_cast<bool>(op1->second);
          if (op2->first == i16_t)
          {
            short operand2 = any_cast<short>(op2->second);
            res->second = (operand1 ? 1 : 0) + operand2;
          }
          else if (op2->first == i32_t)
          {
            int operand2 = any_cast<int>(op2->second);
            res->second = (operand1 ? 1 : 0) + operand2;
          }
          else if (op2->first == i64_t)
          {
            long operand2 = any_cast<long>(op2->second);
            res->second = (operand1 ? 1 : 0) + operand2;
          }
          else if (op2->first == f32_t)
          {
            float operand2 = any_cast<float>(op2->second);
            res->second = (operand1 ? 1 : 0) + operand2;
          }
          else if (op2->first == f64_t)
          {
            double operand2 = any_cast<double>(op2->second);
            res->second = (operand1 ? 1 : 0) + operand2;
          }
          else if (op2->first == str_t)
          {
            string operand2 = any_cast<string>(op2->second);
            res->second = (operand1 ? "true" : "false") + operand2;
          }
          else if (op2->first == bool_t)
          {
            bool operand2 = any_cast<bool>(op2->second);
            res->second = operand1 || operand2;
          }
        }
      }
      if (bo.command == 0x02)
      {
        if (op1->first == i16_t)
        {
          short operand1 = any_cast<short>(op1->second);
          if (op2->first == i16_t)
          {
            short operand2 = any_cast<short>(op2->second);
            res->second = operand1 - operand2;
          }
          else if (op2->first == i32_t)
          {
            int operand2 = any_cast<int>(op2->second);
            res->second = operand1 - operand2;
          }
          else if (op2->first == i64_t)
          {
            long operand2 = any_cast<long>(op2->second);
            res->second = operand1 - operand2;
          }
          else if (op2->first == f32_t)
          {
            float operand2 = any_cast<float>(op2->second);
            res->second = operand1 - operand2;
          }
          else if (op2->first == f64_t)
          {
            double operand2 = any_cast<double>(op2->second);
            res->second = operand1 - operand2;
          }
        }
        else if (op1->first == i32_t)
        {
          int operand1 = any_cast<int>(op1->second);
          if (op2->first == i16_t)
          {
            short operand2 = any_cast<short>(op2->second);
            res->second = operand1 - operand2;
          }
          else if (op2->first == i32_t)
          {
            int operand2 = any_cast<int>(op2->second);
            res->second = operand1 - operand2;
          }
          else if (op2->first == i64_t)
          {
            long operand2 = any_cast<long>(op2->second);
            res->second = operand1 - operand2;
          }
          else if (op2->first == f32_t)
          {
            float operand2 = any_cast<float>(op2->second);
            res->second = operand1 - operand2;
          }
          else if (op2->first == f64_t)
          {
            double operand2 = any_cast<double>(op2->second);
            res->second = operand1 - operand2;
          }
        }
        else if (op1->first == i64_t)
        {
          long operand1 = any_cast<long>(op1->second);
          if (op2->first == i16_t)
          {
            short operand2 = any_cast<short>(op2->second);
            res->second = operand1 - operand2;
          }
          else if (op2->first == i32_t)
          {
            int operand2 = any_cast<int>(op2->second);
            res->second = operand1 - operand2;
          }
          else if (op2->first == i64_t)
          {
            long operand2 = any_cast<long>(op2->second);
            res->second = operand1 - operand2;
          }
          else if (op2->first == f32_t)
          {
            float operand2 = any_cast<float>(op2->second);
            res->second = operand1 - operand2;
          }
          else if (op2->first == f64_t)
          {
            double operand2 = any_cast<double>(op2->second);
            res->second = operand1 - operand2;
          }
        }
        else if (op1->first == f32_t)
        {
          float operand1 = any_cast<float>(op1->second);
          if (op2->first == i16_t)
          {
            short operand2 = any_cast<short>(op2->second);
            res->second = operand1 - operand2;
          }
          else if (op2->first == i32_t)
          {
            int operand2 = any_cast<int>(op2->second);
            res->second = operand1 - operand2;
          }
          else if (op2->first == i64_t)
          {
            long operand2 = any_cast<long>(op2->second);
            res->second = operand1 - operand2;
          }
          else if (op2->first == f32_t)
          {
            float operand2 = any_cast<float>(op2->second);
            res->second = operand1 - operand2;
          }
          else if (op2->first == f64_t)
          {
            double operand2 = any_cast<double>(op2->second);
            res->second = operand1 - operand2;
          }
        }
        else if (op1->first == f64_t)
        {
          double operand1 = any_cast<double>(op1->second);
          if (op2->first == i16_t)
          {
            short operand2 = any_cast<short>(op2->second);
            res->second = operand1 - operand2;
          }
          else if (op2->first == i32_t)
          {
            int operand2 = any_cast<int>(op2->second);
            res->second = operand1 - operand2;
          }
          else if (op2->first == i64_t)
          {
            long operand2 = any_cast<long>(op2->second);
            res->second = operand1 - operand2;
          }
          else if (op2->first == f32_t)
          {
            float operand2 = any_cast<float>(op2->second);
            res->second = operand1 - operand2;
          }
          else if (op2->first == f64_t)
          {
            double operand2 = any_cast<double>(op2->second);
            res->second = operand1 - operand2;
          }
        }
      }
      if (bo.command == 0x03)
      {
        if (op1->first == i16_t)
        {
          short operand1 = any_cast<short>(op1->second);
          if (op2->first == i16_t)
          {
            short operand2 = any_cast<short>(op2->second);
            res->second = operand1 * operand2;
          }
          else if (op2->first == i32_t)
          {
            int operand2 = any_cast<int>(op2->second);
            res->second = operand1 * operand2;
          }
          else if (op2->first == i64_t)
          {
            long operand2 = any_cast<long>(op2->second);
            res->second = operand1 * operand2;
          }
          else if (op2->first == f32_t)
          {
            float operand2 = any_cast<float>(op2->second);
            res->second = operand1 * operand2;
          }
          else if (op2->first == f64_t)
          {
            double operand2 = any_cast<double>(op2->second);
            res->second = operand1 * operand2;
          }
        }
        else if (op1->first == i32_t)
        {
          int operand1 = any_cast<int>(op1->second);
          if (op2->first == i16_t)
          {
            short operand2 = any_cast<short>(op2->second);
            res->second = operand1 * operand2;
          }
          else if (op2->first == i32_t)
          {
            int operand2 = any_cast<int>(op2->second);
            res->second = operand1 * operand2;
          }
          else if (op2->first == i64_t)
          {
            long operand2 = any_cast<long>(op2->second);
            res->second = operand1 * operand2;
          }
          else if (op2->first == f32_t)
          {
            float operand2 = any_cast<float>(op2->second);
            res->second = operand1 * operand2;
          }
          else if (op2->first == f64_t)
          {
            double operand2 = any_cast<double>(op2->second);
            res->second = operand1 * operand2;
          }
        }
        else if (op1->first == i64_t)
        {
          long operand1 = any_cast<long>(op1->second);
          if (op2->first == i16_t)
          {
            short operand2 = any_cast<short>(op2->second);
            res->second = operand1 * operand2;
          }
          else if (op2->first == i32_t)
          {
            int operand2 = any_cast<int>(op2->second);
            res->second = operand1 * operand2;
          }
          else if (op2->first == i64_t)
          {
            long operand2 = any_cast<long>(op2->second);
            res->second = operand1 * operand2;
          }
          else if (op2->first == f32_t)
          {
            float operand2 = any_cast<float>(op2->second);
            res->second = operand1 * operand2;
          }
          else if (op2->first == f64_t)
          {
            double operand2 = any_cast<double>(op2->second);
            res->second = operand1 * operand2;
          }
        }
        else if (op1->first == f32_t)
        {
          float operand1 = any_cast<float>(op1->second);
          if (op2->first == i16_t)
          {
            short operand2 = any_cast<short>(op2->second);
            res->second = operand1 * operand2;
          }
          else if (op2->first == i32_t)
          {
            int operand2 = any_cast<int>(op2->second);
            res->second = operand1 * operand2;
          }
          else if (op2->first == i64_t)
          {
            long operand2 = any_cast<long>(op2->second);
            res->second = operand1 * operand2;
          }
          else if (op2->first == f32_t)
          {
            float operand2 = any_cast<float>(op2->second);
            res->second = operand1 * operand2;
          }
          else if (op2->first == f64_t)
          {
            double operand2 = any_cast<double>(op2->second);
            res->second = operand1 * operand2;
          }
        }
        else if (op1->first == f64_t)
        {
          double operand1 = any_cast<double>(op1->second);
          if (op2->first == i16_t)
          {
            short operand2 = any_cast<short>(op2->second);
            res->second = operand1 * operand2;
          }
          else if (op2->first == i32_t)
          {
            int operand2 = any_cast<int>(op2->second);
            res->second = operand1 * operand2;
          }
          else if (op2->first == i64_t)
          {
            long operand2 = any_cast<long>(op2->second);
            res->second = operand1 * operand2;
          }
          else if (op2->first == f32_t)
          {
            float operand2 = any_cast<float>(op2->second);
            res->second = operand1 * operand2;
          }
          else if (op2->first == f64_t)
          {
            double operand2 = any_cast<double>(op2->second);
            res->second = operand1 * operand2;
          }
        }
      }
      if (bo.command == 0x04)
      {
        if (op1->first == i16_t)
        {
          short operand1 = any_cast<short>(op1->second);
          if (op2->first == i16_t)
          {
            short operand2 = any_cast<short>(op2->second);
            res->second = operand1 / operand2;
          }
          else if (op2->first == i32_t)
          {
            int operand2 = any_cast<int>(op2->second);
            res->second = operand1 / operand2;
          }
          else if (op2->first == i64_t)
          {
            long operand2 = any_cast<long>(op2->second);
            res->second = operand1 / operand2;
          }
          else if (op2->first == f32_t)
          {
            float operand2 = any_cast<float>(op2->second);
            res->second = operand1 / operand2;
          }
          else if (op2->first == f64_t)
          {
            double operand2 = any_cast<double>(op2->second);
            res->second = operand1 / operand2;
          }
        }
        else if (op1->first == i32_t)
        {
          int operand1 = any_cast<int>(op1->second);
          if (op2->first == i16_t)
          {
            short operand2 = any_cast<short>(op2->second);
            res->second = operand1 / operand2;
          }
          else if (op2->first == i32_t)
          {
            int operand2 = any_cast<int>(op2->second);
            res->second = operand1 / operand2;
          }
          else if (op2->first == i64_t)
          {
            long operand2 = any_cast<long>(op2->second);
            res->second = operand1 / operand2;
          }
          else if (op2->first == f32_t)
          {
            float operand2 = any_cast<float>(op2->second);
            res->second = operand1 / operand2;
          }
          else if (op2->first == f64_t)
          {
            double operand2 = any_cast<double>(op2->second);
            res->second = operand1 / operand2;
          }
        }
        else if (op1->first == i64_t)
        {
          long operand1 = any_cast<long>(op1->second);
          if (op2->first == i16_t)
          {
            short operand2 = any_cast<short>(op2->second);
            res->second = operand1 / operand2;
          }
          else if (op2->first == i32_t)
          {
            int operand2 = any_cast<int>(op2->second);
            res->second = operand1 / operand2;
          }
          else if (op2->first == i64_t)
          {
            long operand2 = any_cast<long>(op2->second);
            res->second = operand1 / operand2;
          }
          else if (op2->first == f32_t)
          {
            float operand2 = any_cast<float>(op2->second);
            res->second = operand1 / operand2;
          }
          else if (op2->first == f64_t)
          {
            double operand2 = any_cast<double>(op2->second);
            res->second = operand1 / operand2;
          }
        }
        else if (op1->first == f32_t)
        {
          float operand1 = any_cast<float>(op1->second);
          if (op2->first == i16_t)
          {
            short operand2 = any_cast<short>(op2->second);
            res->second = operand1 / operand2;
          }
          else if (op2->first == i32_t)
          {
            int operand2 = any_cast<int>(op2->second);
            res->second = operand1 / operand2;
          }
          else if (op2->first == i64_t)
          {
            long operand2 = any_cast<long>(op2->second);
            res->second = operand1 / operand2;
          }
          else if (op2->first == f32_t)
          {
            float operand2 = any_cast<float>(op2->second);
            res->second = operand1 / operand2;
          }
          else if (op2->first == f64_t)
          {
            double operand2 = any_cast<double>(op2->second);
            res->second = operand1 / operand2;
          }
        }
        else if (op1->first == f64_t)
        {
          double operand1 = any_cast<double>(op1->second);
          if (op2->first == i16_t)
          {
            short operand2 = any_cast<short>(op2->second);
            res->second = operand1 / operand2;
          }
          else if (op2->first == i32_t)
          {
            int operand2 = any_cast<int>(op2->second);
            res->second = operand1 / operand2;
          }
          else if (op2->first == i64_t)
          {
            long operand2 = any_cast<long>(op2->second);
            res->second = operand1 / operand2;
          }
          else if (op2->first == f32_t)
          {
            float operand2 = any_cast<float>(op2->second);
            res->second = operand1 / operand2;
          }
          else if (op2->first == f64_t)
          {
            double operand2 = any_cast<double>(op2->second);
            res->second = operand1 / operand2;
          }
        }
      }
      if (bo.command == 0x06)
      {
        if (op1->first == i16_t)
        {
          short operand1 = any_cast<short>(op1->second);
          if (op2->first == i16_t)
          {
            short operand2 = any_cast<short>(op2->second);
            res->second = operand1 ^ operand2;
          }
          else if (op2->first == i32_t)
          {
            int operand2 = any_cast<int>(op2->second);
            res->second = operand1 ^ operand2;
          }
          else if (op2->first == i64_t)
          {
            long operand2 = any_cast<long>(op2->second);
            res->second = operand1 ^ operand2;
          }
          else if (op2->first == f32_t)
          {
            float operand2 = any_cast<float>(op2->second);
            res->second = operand1 ^ ((long)operand2);
          }
          else if (op2->first == f64_t)
          {
            double operand2 = any_cast<double>(op2->second);
            res->second = operand1 ^ ((long)operand2);
          }
        }
        else if (op1->first == i32_t)
        {
          int operand1 = any_cast<int>(op1->second);
          if (op2->first == i16_t)
          {
            short operand2 = any_cast<short>(op2->second);
            res->second = operand1 ^ operand2;
          }
          else if (op2->first == i32_t)
          {
            int operand2 = any_cast<int>(op2->second);
            res->second = operand1 ^ operand2;
          }
          else if (op2->first == i64_t)
          {
            long operand2 = any_cast<long>(op2->second);
            res->second = operand1 ^ operand2;
          }
          else if (op2->first == f32_t)
          {
            float operand2 = any_cast<float>(op2->second);
            res->second = operand1 ^ ((long)operand2);
          }
          else if (op2->first == f64_t)
          {
            double operand2 = any_cast<double>(op2->second);
            res->second = operand1 ^ ((long)operand2);
          }
        }
        else if (op1->first == i64_t)
        {
          long operand1 = any_cast<long>(op1->second);
          if (op2->first == i16_t)
          {
            short operand2 = any_cast<short>(op2->second);
            res->second = operand1 ^ operand2;
          }
          else if (op2->first == i32_t)
          {
            int operand2 = any_cast<int>(op2->second);
            res->second = operand1 ^ operand2;
          }
          else if (op2->first == i64_t)
          {
            long operand2 = any_cast<long>(op2->second);
            res->second = operand1 ^ operand2;
          }
          else if (op2->first == f32_t)
          {
            float operand2 = any_cast<float>(op2->second);
            res->second = operand1 ^ ((long)operand2);
          }
          else if (op2->first == f64_t)
          {
            double operand2 = any_cast<double>(op2->second);
            res->second = operand1 ^ ((long)operand2);
          }
        }
        else if (op1->first == f32_t)
        {
          float operand1 = any_cast<float>(op1->second);
          if (op2->first == i16_t)
          {
            short operand2 = any_cast<short>(op2->second);
            res->second = ((long)operand1) ^ operand2;
          }
          else if (op2->first == i32_t)
          {
            int operand2 = any_cast<int>(op2->second);
            res->second = ((long)operand1) ^ operand2;
          }
          else if (op2->first == i64_t)
          {
            long operand2 = any_cast<long>(op2->second);
            res->second = ((long)operand1) ^ operand2;
          }
          else if (op2->first == f32_t)
          {
            float operand2 = any_cast<float>(op2->second);
            res->second = ((long)operand1) ^ ((long)operand2);
          }
          else if (op2->first == f64_t)
          {
            double operand2 = any_cast<double>(op2->second);
            res->second = ((long)operand1) ^ ((long)operand2);
          }
        }
        else if (op1->first == f64_t)
        {
          double operand1 = any_cast<double>(op1->second);
          if (op2->first == i16_t)
          {
            short operand2 = any_cast<short>(op2->second);
            res->second = ((long)operand1) ^ operand2;
          }
          else if (op2->first == i32_t)
          {
            int operand2 = any_cast<int>(op2->second);
            res->second = ((long)operand1) ^ operand2;
          }
          else if (op2->first == i64_t)
          {
            long operand2 = any_cast<long>(op2->second);
            res->second = ((long)operand1) ^ operand2;
          }
          else if (op2->first == f32_t)
          {
            float operand2 = any_cast<float>(op2->second);
            res->second = ((long)operand1) ^ ((long)operand2);
          }
          else if (op2->first == f64_t)
          {
            double operand2 = any_cast<double>(op2->second);
            res->second = ((long)operand1) ^ ((long)operand2);
          }
        }
      }
      if (bo.command == 0x05)
      {
        if (op1->first == i16_t)
        {
          short operand1 = any_cast<short>(op1->second);
          if (op2->first == i16_t)
          {
            short operand2 = any_cast<short>(op2->second);
            res->second = operand1 % operand2;
          }
          else if (op2->first == i32_t)
          {
            int operand2 = any_cast<int>(op2->second);
            res->second = operand1 % operand2;
          }
          else if (op2->first == i64_t)
          {
            long operand2 = any_cast<long>(op2->second);
            res->second = operand1 % operand2;
          }
          else if (op2->first == f32_t)
          {
            float operand2 = any_cast<float>(op2->second);
            res->second = operand1 % ((long)operand2);
          }
          else if (op2->first == f64_t)
          {
            double operand2 = any_cast<double>(op2->second);
            res->second = operand1 % ((long)operand2);
          }
        }
        else if (op1->first == i32_t)
        {
          int operand1 = any_cast<int>(op1->second);
          if (op2->first == i16_t)
          {
            short operand2 = any_cast<short>(op2->second);
            res->second = operand1 % operand2;
          }
          else if (op2->first == i32_t)
          {
            int operand2 = any_cast<int>(op2->second);
            res->second = operand1 % operand2;
          }
          else if (op2->first == i64_t)
          {
            long operand2 = any_cast<long>(op2->second);
            res->second = operand1 % operand2;
          }
          else if (op2->first == f32_t)
          {
            float operand2 = any_cast<float>(op2->second);
            res->second = operand1 % ((long)operand2);
          }
          else if (op2->first == f64_t)
          {
            double operand2 = any_cast<double>(op2->second);
            res->second = operand1 % ((long)operand2);
          }
        }
        else if (op1->first == i64_t)
        {
          long operand1 = any_cast<long>(op1->second);
          if (op2->first == i16_t)
          {
            short operand2 = any_cast<short>(op2->second);
            res->second = operand1 % operand2;
          }
          else if (op2->first == i32_t)
          {
            int operand2 = any_cast<int>(op2->second);
            res->second = operand1 % operand2;
          }
          else if (op2->first == i64_t)
          {
            long operand2 = any_cast<long>(op2->second);
            res->second = operand1 % operand2;
          }
          else if (op2->first == f32_t)
          {
            float operand2 = any_cast<float>(op2->second);
            res->second = operand1 % ((long)operand2);
          }
          else if (op2->first == f64_t)
          {
            double operand2 = any_cast<double>(op2->second);
            res->second = operand1 % ((long)operand2);
          }
        }
        else if (op1->first == f32_t)
        {
          float operand1 = any_cast<float>(op1->second);
          if (op2->first == i16_t)
          {
            short operand2 = any_cast<short>(op2->second);
            res->second = ((long)operand1) % operand2;
          }
          else if (op2->first == i32_t)
          {
            int operand2 = any_cast<int>(op2->second);
            res->second = ((long)operand1) % operand2;
          }
          else if (op2->first == i64_t)
          {
            long operand2 = any_cast<long>(op2->second);
            res->second = ((long)operand1) % operand2;
          }
          else if (op2->first == f32_t)
          {
            float operand2 = any_cast<float>(op2->second);
            res->second = ((long)operand1) % ((long)operand2);
          }
          else if (op2->first == f64_t)
          {
            double operand2 = any_cast<double>(op2->second);
            res->second = ((long)operand1) % ((long)operand2);
          }
        }
        else if (op1->first == f64_t)
        {
          double operand1 = any_cast<double>(op1->second);
          if (op2->first == i16_t)
          {
            short operand2 = any_cast<short>(op2->second);
            res->second = ((long)operand1) % operand2;
          }
          else if (op2->first == i32_t)
          {
            int operand2 = any_cast<int>(op2->second);
            res->second = ((long)operand1) % operand2;
          }
          else if (op2->first == i64_t)
          {
            long operand2 = any_cast<long>(op2->second);
            res->second = ((long)operand1) % operand2;
          }
          else if (op2->first == f32_t)
          {
            float operand2 = any_cast<float>(op2->second);
            res->second = ((long)operand1) % ((long)operand2);
          }
          else if (op2->first == f64_t)
          {
            double operand2 = any_cast<double>(op2->second);
            res->second = ((long)operand1) % ((long)operand2);
          }
        }
      }
      res->first = checkTypeOfAny(res->second);
    }
    else if (exp.first == 0xf9)
    {
      MemberExp mexp = any_cast<MemberExp>(exp.second);
      auto raw = createEmptyUnit();
      resolveValue(mexp.object, false, raw);
      Ref ref = any_cast<Ref>(raw->second);
      if (raw->first == arr_t)
      {
        Array *arr = (Array *)ref.pointer;
        auto a = createEmptyUnit();
        resolveValue(mexp.property, false, a);
        int index = any_cast<int>(a->second);
        if (index < 0 || index >= arr->items.size())
        {
          *res = {und_t, NULL};
        }
        else
        {
          *res = *arr->items[index];
        }
      }
      else if (raw->first == obj_t)
      {
        string prop = "";
        if (mexp.property.first == iden_t)
        {
          Id id = any_cast<Id>(mexp.property.second);
          prop = id.name;
        }
        else
        {
          auto a = createEmptyUnit();
          resolveValue(mexp.property, false, a);
          prop = any_cast<std::string>(a->second);
        }
        Object *obj = (Object *)ref.pointer;
        *res = *obj->props[prop];
      }
    }
    else if (exp.first == 0xfc)
    {
      Call cexp = any_cast<Call>(exp.second);
      pair<uint32_t, any> *fnHolder = createEmptyUnit();
      resolveValue(cexp.callee, false, fnHolder);
      Fn fn = any_cast<Fn>(fnHolder->second);
      this->mem.pushLayer(this->pc, this->end, res);
      for (int i = 0; i < fn.params.size(); i++)
      {
        auto paramName = any_cast<Id>(fn.params[i].second).name;
        auto paramValue = createEmptyUnit();
        resolveValue(cexp.args[i], false, paramValue);
        this->mem.add(paramName, paramValue);
      }
      this->prepare(fn.startPos, fn.startPos + fn.length, NULL);
    }
    else
    {
      *res = {exp.first, exp.second};
    }
  }
  void prepareLooper()
  {
    this->looper = thread([this]
                          {
      while (!stop_) {
        function<void()> task;
        {
          unique_lock<mutex> lock(
            this->queue_mutex_);
          cv_.wait(lock, [this] {
            return !this->tasks.empty() || this->stop_;
          });
          if (this->stop_ && this->tasks.empty()) {
            return;
          }
          task = this->tasks.front();
          this->tasks.pop();
        }
        printf("executing...\n");
        task();
        printf("done!\n");
      }
      printf("ended!\n"); });
  }
  Runtime(char *vmId, int index, unsigned char cd[], uint64_t length,
          bool (*areAllPassed)(uint64_t instCounter, std::unordered_map<char *, uint32_t> waiters, char *myId),
          void (*tryToCheckTrigger)(char *vmId, uint32_t resNum, uint64_t instNum, char *myId),
          int (*lock)(char *vmId, uint32_t resNum, bool shouldLock),
          void (*unlock)(char *vmId, uint32_t resNum))
  {
    this->onchain = true;
    this->id = vmId;
    this->index = index;
    this->areAllPassed = areAllPassed;
    this->tryToCheckTrigger = tryToCheckTrigger;
    this->lock = lock;
    this->unlock = unlock;
    this->code = vector<pair<unsigned char, Operation *> *>(length, NULL);
    this->rawCode = vector<unsigned char>(length, 0);
    this->jumps = vector<uint32_t>(length, 0);
    for (int i = 0; i < length; i++)
    {
      this->code[i] = new pair<unsigned char, Operation *>(cd[i], NULL);
      this->rawCode[i] = cd[i];
    }
    this->mem.pushLayer(0, length);
    this->loadDefaultTypes();
    this->loadApi();
    this->prepareLooper();
    // this->registerHost();
  }
  Runtime(char *vmId, int index, Operation *cd[], uint64_t length,
          bool (*areAllPassed)(uint64_t instCounte, std::unordered_map<char *, uint32_t> waiterrs, char *myId),
          void (*tryToCheckTrigger)(char *vmId, uint32_t resNum, uint64_t instNum, char *myId),
          int (*lock)(char *vmId, uint32_t resNum, bool shouldLock),
          void (*unlock)(char *vmId, uint32_t resNum))
  {
    this->onchain = true;
    this->id = vmId;
    this->index = index;
    this->areAllPassed = areAllPassed;
    this->tryToCheckTrigger = tryToCheckTrigger;
    this->lock = lock;
    this->unlock = unlock;
    this->code = vector<pair<unsigned char, Operation *> *>(length, NULL);
    this->rawCode = vector<unsigned char>(length, 0);
    this->jumps = vector<uint32_t>(length, 0);
    for (int i = 0; i < length; i++)
    {
      this->code[i] = new pair<unsigned char, Operation *>(cd[i]->opcode, cd[i]);
      this->rawCode[i] = cd[i]->opcode;
    }
    this->mem.pushLayer(0, length);
    this->loadDefaultTypes();
    this->loadApi();
    this->prepareLooper();
    // this->registerHost();
  }
  Runtime(unsigned char cd[], uint64_t length, function<char *(char *)> cb)
  {
    this->onchain = false;
    this->callback = cb;
    send = cb;
    this->id = "";
    this->index = 0;
    this->code = vector<pair<unsigned char, Operation *> *>(length + 1 + 1 + 1, NULL);
    this->rawCode = vector<unsigned char>(length + 1 + 1 + 1, 0);
    this->jumps = vector<uint32_t>(length + 1 + 1 + 1, 0);
    this->code[0] = new pair<unsigned char, Operation *>(0x00, NULL);
    this->rawCode[0] = 0x00;
    for (auto i = 1; i < 1 + 1; i++)
    {
      this->code[i] = new pair<unsigned char, Operation *>(0x00, NULL);
      this->rawCode[i] = 0x00;
    }
    for (int i = 1 + 1; i < length + 1 + 1; i++)
    {
      this->code[i] = new pair<unsigned char, Operation *>(cd[i - (1 + 1)], NULL);
      this->rawCode[i] = cd[i - (1 + 1)];
    }
    this->code[length + 1 + 1] = new pair<unsigned char, Operation *>(0xff, NULL);
    this->rawCode[length + 1 + 1] = 0xff;
    this->mem.pushLayer(0, length);
    this->loadDefaultTypes();
    this->loadApi();
    this->prepareLooper();
  }
  Runtime(Operation *cd[], uint64_t length)
  {
    this->onchain = false;
    this->id = "";
    this->index = 0;
    this->code = vector<pair<unsigned char, Operation *> *>(length, NULL);
    this->rawCode = vector<unsigned char>(length, 0);
    this->jumps = vector<uint32_t>(length, 0);
    for (int i = 0; i < length; i++)
    {
      this->code[i] = new pair<unsigned char, Operation *>(cd[i]->opcode, cd[i]);
      this->rawCode[i] = cd[i]->opcode;
    }
    this->mem.pushLayer(0, length);
    this->loadDefaultTypes();
    this->loadApi();
    this->prepareLooper();
  }

  void loadApi()
  {
    this->mem.add("console", new pair<uint32_t, any>(obj_t, Ref{1, new Object({{"log", new pair<uint32_t, any>(fn_t, Fn("", {{iden_t, Id("text")}}, 1, 1))}})}));
  }

  void registerHost()
  {
    WasmEdge_ConfigureContext *ConfCxt = WasmEdge_ConfigureCreate();
    WasmEdge_ConfigureAddHostRegistration(ConfCxt, WasmEdge_HostRegistration_Wasi);
    WasmEdge_ConfigureStatisticsSetInstructionCounting(ConfCxt, true);
    WasmEdge_VMContext *VMCxt = WasmEdge_VMCreate(ConfCxt, NULL);

    WasmEdge_String HostName = WasmEdge_StringCreateByCString("env");
    WasmEdge_ModuleInstanceContext *HostMod = WasmEdge_ModuleInstanceCreate(HostName);
    this->registerFunction(HostMod, "lock", WasmLock);
    this->registerFunction(HostMod, "unlock", WasmUnlock);
    auto Res = WasmEdge_VMRegisterModuleFromImport(VMCxt, HostMod);
    if (!WasmEdge_ResultOK(Res))
    {
      printf("Host module registration failed: %s\n", WasmEdge_ResultGetMessage(Res));
    }
    WasmEdge_StringDelete(HostName);

    Res = WasmEdge_VMLoadWasmFromFile(VMCxt, "main.wasm");
    if (!WasmEdge_ResultOK(Res))
    {
      printf("Loading phase failed: %s\n", WasmEdge_ResultGetMessage(Res));
    }
    Res = WasmEdge_VMValidate(VMCxt);
    if (!WasmEdge_ResultOK(Res))
    {
      printf("Validation phase failed: %s\n", WasmEdge_ResultGetMessage(Res));
    }
    Res = WasmEdge_VMInstantiate(VMCxt);
    if (!WasmEdge_ResultOK(Res))
    {
      printf("Instantiation phase failed: %s\n", WasmEdge_ResultGetMessage(Res));
    }

    this->vm = VMCxt;
  }

  void registerFunction(WasmEdge_ModuleInstanceContext *HostMod, char *name, WasmEdge_HostFunc_t fn)
  {
    WasmEdge_ValType ParamList[1] = {WasmEdge_ValTypeGenI32()};
    WasmEdge_ValType ReturnList[1] = {WasmEdge_ValTypeGenI32()};
    WasmEdge_FunctionTypeContext *HostFType = WasmEdge_FunctionTypeCreate(ParamList, 1, ReturnList, 1);
    WasmEdge_FunctionInstanceContext *HostFunc = WasmEdge_FunctionInstanceCreate(HostFType, fn, this->id, 0);
    WasmEdge_String HostName = WasmEdge_StringCreateByCString(name);
    WasmEdge_ModuleInstanceAddFunction(HostMod, HostName, HostFunc);
    WasmEdge_StringDelete(HostName);
    WasmEdge_FunctionTypeDelete(HostFType);
  }

  void enqueue(function<void()> task)
  {
    {
      unique_lock<std::mutex> lock(queue_mutex_);
      if (this->tasks.empty())
      {
        // printf("enqueing...\n");
        this->tasks.emplace(task);
      }
    }
    cv_.notify_one();
  }

  void prepare(size_t start, size_t end, void *result)
  {
    this->result = result;
    this->pc = start;
    this->end = end;
  }

  void executeOnUpdate(const char *sendType, const char *pointId, const char *userId,
                       Ref input, bool isCritical = false)
  {
    this->mem.add("sendType", new pair<uint32_t, any>(str_t, string(sendType)));
    this->mem.add("pointId", new pair<uint32_t, any>(str_t, string(pointId)));
    this->mem.add("userId", new pair<uint32_t, any>(str_t, string(userId)));
    this->mem.add("inputData", new pair<uint32_t, any>(obj_t, input));
    this->execute(isCritical);
  }

  void executeWithInput(Ref input, bool isCritical = false)
  {
    this->mem.add("input", new pair<uint32_t, any>(obj_t, input));
    this->execute(isCritical);
  }

  void execute(bool isCritical = false)
  {
    this->paused = false;
    uint64_t lockStart = 0;
    std::string lockName;
    vector<std::string> lockResNums{};
    while (this->pc != this->end)
    {
      if (this->paused)
      {
        break;
      }
      this->instCounter++;
      uint64_t opPos = this->pc;
      auto c = this->code[this->pc];
      this->pc++;
      Operation *rawop = c != NULL ? c->second : NULL;
      switch (c->first)
      {
      case 0x00:
      {
        if (opPos == 1)
        {
          auto t = createEmptyUnit();
          resolveValue({iden_t, Id("text")}, false, t);
          std::string value = Utils::toString(t);
          std::string text = "{\"key\":\"log\", \"input\": {\"text\": \"" + value + "\"}}";
          this->callback(&text[0]);
        }
        break;
      }
      case 0xff:
      {
        this->pc = 0;
        break;
      }
      case DefineVar::OPCODE:
      {
        if (rawop == NULL)
        {
          auto nameLength = Utils::parseDataAsInt(Utils::pickSubarray(this->rawCode, this->pc, 4));
          this->pc += 4;
          auto name = Utils::pickString(this->rawCode, this->pc, nameLength);
          this->pc += nameLength;
          auto valData = pickVal(this->mem, this->types, this->rawCode, this->pc);
          this->pc = valData.second;
          rawop = new DefineVar(name, valData.first.first, valData.first.second);
          this->code[opPos]->second = rawop;
          this->jumps[opPos] = this->pc;
        }
        else
        {
          this->pc = this->jumps[opPos];
        }
        if (!recording)
        {
          auto op = (DefineVar *)rawop;
          std::string ok = std::string("ok");
          auto unit = new pair<uint32_t, any>(str_t, ok);
          this->mem.add(op->name, unit);
          resolveValue(*op->value, false, unit);
        }
        break;
      }
      case ExpOp::OPCODE:
      {
        if (rawop == NULL)
        {
          auto exp = pickVal(mem, types, this->rawCode, this->pc);
          this->pc = exp.second;
          rawop = new ExpOp(exp.first);
          this->code[opPos]->second = rawop;
          this->jumps[opPos] = this->pc;
        }
        else
        {
          this->pc = this->jumps[opPos];
        }
        if (!recording)
        {
          auto op = (ExpOp *)rawop;
          auto r = createEmptyUnit();
          this->resolveValue(op->exp, false, r);
        }
        break;
      }
      case DefineFunc::OPCODE:
      {
        if (rawop == NULL)
        {
          auto nameLength = Utils::parseDataAsInt(Utils::pickSubarray(this->rawCode, this->pc, 4));
          this->pc += 4;
          auto name = Utils::pickString(this->rawCode, this->pc, nameLength);
          this->pc += nameLength;
          auto paramsCount = Utils::parseDataAsInt(Utils::pickSubarray(this->rawCode, this->pc, 4));
          this->pc += 4;
          vector<pair<uint32_t, any>> arr{};
          for (int i = 0; i < paramsCount; i++)
          {
            auto parsedItem = pickVal(mem, types, this->rawCode, this->pc);
            arr.push_back({parsedItem.first.first, parsedItem.first.second});
            this->pc = parsedItem.second;
          }
          uint32_t length = Utils::parseDataAsInt(Utils::pickSubarray(this->rawCode, this->pc, 4));
          this->pc += 4;
          uint32_t startPos = this->pc;
          this->pc += length;
          auto fn = Fn(name, arr, startPos, length);
          rawop = new DefineFunc(fn);
          this->code[opPos]->second = rawop;
          this->jumps[opPos] = this->pc;
        }
        else
        {
          this->pc = this->jumps[opPos];
        }
        if (!recording)
        {
          auto op = (DefineFunc *)rawop;
          this->mem.add(op->func.id, new pair<uint32_t, any>(fn_t, op->func));
        }
        break;
      }
      case ReturnOp::OPCODE:
      {
        if (rawop == NULL)
        {
          auto valData = pickVal(this->mem, this->types, this->rawCode, this->pc);
          this->pc = valData.second;
          rawop = new ReturnOp(valData.first);
          this->code[opPos]->second = rawop;
          this->jumps[opPos] = this->pc;
        }
        else
        {
          this->pc = this->jumps[opPos];
        }
        if (!recording)
        {
          auto op = (ReturnOp *)rawop;
          auto unit = createEmptyUnit();
          resolveValue(op->arg, false, unit);
          this->mem.setAsReturnVal(unit);
        }
        break;
      }
      case VarPlusPlus::OPCODE:
      {
        if (rawop == NULL)
        {
          auto name = pickVal(this->mem, this->types, this->rawCode, this->pc);
          this->pc += name.second;
          rawop = new VarPlusPlus(name.first);
          this->code[opPos]->second = rawop;
          this->jumps[opPos] = this->pc;
        }
        else
        {
          this->pc = this->jumps[opPos];
        }
        if (!recording)
        {
          auto op = (VarPlusPlus *)rawop;
          auto v = createEmptyUnit();
          resolveValue(op->name, false, v);
          any res = Utils::modifyNumber(v->first, v->second, +1);
          auto val = createEmptyUnit();
          resolveValue(op->name, false, val);
          val->first = checkTypeOfAny(res);
          val->second = res;
        }
        break;
      }
      case VarMinMin::OPCODE:
      {
        if (rawop == NULL)
        {
          auto name = pickVal(this->mem, this->types, this->rawCode, this->pc);
          this->pc += name.second;
          rawop = new VarMinMin(name.first);
          this->code[opPos]->second = rawop;
          this->jumps[opPos] = this->pc;
        }
        else
        {
          this->pc = this->jumps[opPos];
        }
        if (!recording)
        {
          auto op = (VarMinMin *)rawop;
          auto v = createEmptyUnit();
          resolveValue(op->name, false, v);
          any res = Utils::modifyNumber(v->first, v->second, -1);
          auto val = createEmptyUnit();
          resolveValue(op->name, false, val);
          val->first = checkTypeOfAny(res);
          val->second = res;
        }
        break;
      }
      case LockData::OPCODE:
      {
        if (rawop == NULL)
        {
          auto lockNameLength = Utils::parseDataAsInt(Utils::pickSubarray(this->rawCode, this->pc, 4));
          this->pc += 4;
          std::string lockName = Utils::pickString(this->rawCode, this->pc, lockNameLength);
          this->pc += lockNameLength;
          uint32_t resCount = Utils::parseDataAsInt(Utils::pickSubarray(this->rawCode, this->pc, 4));
          this->pc += 4;
          vector<std::string> resNums{};
          for (int i = 0; i < resCount; i++)
          {
            auto resNumLength = Utils::parseDataAsInt(Utils::pickSubarray(this->rawCode, this->pc, 4));
            this->pc += 4;
            std::string resNum = Utils::pickString(this->rawCode, this->pc, resNumLength);
            this->pc += resNumLength;
            resNums.push_back(resNum);
          }
          rawop = new LockData(lockName, resNums);
          this->code[opPos]->second = rawop;
          this->jumps[opPos] = this->pc;
        }
        else
        {
          this->pc = this->jumps[opPos];
        }
        if (!isCritical)
        {
          auto op = (LockData *)rawop;
          lockName = op->name;
          lockResNums = op->keys;
          lockStart = pc - 1;
          recording = true;
          // this->lock(this->id, op->key, true);
        }
        break;
      }
      case UnlockData::OPCODE:
      {
        if (rawop == NULL)
        {
          auto lockNameLength = Utils::parseDataAsInt(Utils::pickSubarray(this->rawCode, this->pc, 4));
          this->pc += 4;
          std::string lockName = Utils::pickString(this->rawCode, this->pc, lockNameLength);
          this->pc += lockNameLength;
          uint32_t resCount = Utils::parseDataAsInt(Utils::pickSubarray(this->rawCode, this->pc, 4));
          this->pc += 4;
          vector<std::string> resNums{};
          for (int i = 0; i < resCount; i++)
          {
            auto resNumLength = Utils::parseDataAsInt(Utils::pickSubarray(this->rawCode, this->pc, 4));
            this->pc += 4;
            std::string resNum = Utils::pickString(this->rawCode, this->pc, resNumLength);
            this->pc += resNumLength;
            resNums.push_back(resNum);
          }
          rawop = new UnlockData(lockName, resNums);
          this->code[opPos]->second = rawop;
          this->jumps[opPos] = this->pc;
        }
        else
        {
          this->pc = this->jumps[opPos];
        }
        if (!isCritical)
        {
          auto op = (UnlockData *)rawop;
          syncTasks.push_back({lockResNums, lockStart + 1, pc - 1, lockName});
          lockStart = 0;
          lockResNums = {};
          recording = false;
          // this->unlock(this->id, op->key);
        }
        break;
      }
      default:
      {
        std::string text = "{\"key\":\"log\", \"input\": {\"text\": \"unknown opcode: " + to_string(this->pc) + ":" + to_string(c->first) + "\"}}";
        this->callback(&text[0]);
        return;
      }
      }
      while ((this->mem.retPoints.size() > 0) && (this->pc == this->end))
      {
        pair<size_t, size_t> ret = this->mem.popLayer();
        this->pc = ret.first;
        this->end = ret.second;
        if (this->mem.retPoints.size() == 0)
        {
          this->pc = 0;
          break;
        }
      }
      if (this->pc == 0)
      {
        if (this->onchain)
        {
          if (!isCritical)
          {
            globalLock.lock();
            doneTasks++;
            if (doneTasks == COUNT)
            {
              if (step == 0)
              {
                doneTasks = 0;
                step++;
                globalLock.unlock();
                doCritical();
              }
            }
            else
            {
              globalLock.unlock();
            }
            break;
          }
        }
        else
        {
          break;
        }
      }
    }
    // std::string text = "{\"key\":\"log\", \"input\": {\"text\": \"" +
    //                    any_cast<std::string>(this->mem.get("a")->second) +
    //                    "\"}}";
    // this->callback(&text[0]);
  }

  void run(size_t start, size_t end, void *result)
  {
    this->prepare(start, end, result);
    this->enqueue([this]
                  {
                    this->execute();
                    // WasmEdge_String FuncName = WasmEdge_StringCreateByCString("run");
                    // WasmEdge_Value Params[1] = {WasmEdge_ValueGenI32((this->index % 4) + 1)};
                    // WasmEdge_Value Returns[0];
                    // auto Res = WasmEdge_VMExecuteWithNotifier(this->vm, FuncName, Params, 1, Returns, 0, this->index, isPaused, notifyInstCount, check);
                    // if (!WasmEdge_ResultOK(Res)) {
                    //   printf("Execution phase failed: %s\n", WasmEdge_ResultGetMessage(Res));
                    // }
                    // WasmEdge_StringDelete(FuncName);
                  });
  }

  void pause()
  {
    // printf("pausing...");
    if (!this->paused)
    {
      this->paused = true;
    }
  }

  void resume(uint64_t instNum)
  {
    // printf("resuming...\n");
    this->enqueue([this]
                  {
                    this->execute();

                    // this->paused = false;
                    // WasmEdge_String FuncName = WasmEdge_StringCreateByCString("run");
                    // WasmEdge_Value Params[1] = {WasmEdge_ValueGenI32((this->index % 4) + 1)};
                    // WasmEdge_Value Returns[0];
                    // auto Res = WasmEdge_VMConExecuteWithNotifier(this->vm, FuncName, Params, 1, Returns, 0, this->index, isPaused, notifyInstCount, check);
                    // if (!WasmEdge_ResultOK(Res)) {
                    //   printf("Execution phase failed: %s\n", WasmEdge_ResultGetMessage(Res));
                    // }
                    // WasmEdge_StringDelete(FuncName);
                  });
  }

  void addTrigger(uint64_t num, char *vmId, uint32_t resNum)
  {
    // this->tirggerLock.lock();
    // printf("planting trigger for %S at %d\n", vmId, num);
    if (auto trigger = this->triggerListeners.find(num); trigger != this->triggerListeners.end())
    {
      trigger->second[vmId] = resNum;
    }
    else
    {
      std::unordered_map<char *, uint32_t> m{{vmId, resNum}};
      this->triggerListeners[num] = m;
      this->triggerQueue.push(num);
    }
    this->newTiggersPendingToAdd = true;
    this->triggerFront = this->triggerQueue.top();
    // this->tirggerLock.unlock();
  }

  void stick()
  {
    this->looper.join();
  }
};

std::unordered_map<char *, std::pair<int, Runtime *>> vmMap = {};

WasmEdge_Result WasmLock(void *data, const WasmEdge_CallingFrameContext *, const WasmEdge_Value *In, WasmEdge_Value *Out)
{
  uint32_t resNum = WasmEdge_ValueGetI32(In[0]);
  auto vmId = (char *)data;
  vmMap[vmId].second->lock(vmId, resNum, true);
  return WasmEdge_Result_Success;
}

WasmEdge_Result WasmUnlock(void *data, const WasmEdge_CallingFrameContext *, const WasmEdge_Value *In, WasmEdge_Value *Out)
{
  uint32_t resNum = WasmEdge_ValueGetI32(In[0]);
  auto vmId = (char *)data;
  vmMap[vmId].second->unlock(vmId, resNum);
  return WasmEdge_Result_Success;
}

Runtime *vms[COUNT];

bool isPaused(int index)
{
  return vms[index]->paused;
}

void notifyInstCount(int index, uint64_t instNum)
{
  if (instNum == 0)
  {
    vms[index]->stop_ = true;
    globalLock.lock();
    doneTasks++;
    if (doneTasks == COUNT)
    {
      endProgram();
    }
    globalLock.unlock();
  }
  else
  {
    vms[index]->instCounter = instNum;
  }
}

void check(int index)
{
  Runtime *rt = vms[index];
  if (rt->newTiggersPendingToAdd && (rt->triggerFront == rt->instCounter || rt->triggerFront == (rt->instCounter + 1) || rt->triggerFront == (rt->instCounter + 2)))
  {
    // rt->tirggerLock.lock();
    if (!rt->triggerQueue.empty())
    {
      auto t = rt->triggerQueue.top();
      // printf("checking point : %d %d\n", t, rt->instCounter);
      while (rt->instCounter + 1 >= t)
      {
        // printf("%s %d %d \n", rt->id, rt->instCounter, rt->triggerFront.load(std::memory_order_relaxed));
        if (rt->areAllPassed(t, rt->triggerListeners[t], rt->id))
        {
          // printf("trigger reached %d by %s\n", t, rt->id);
          auto trigger = rt->triggerListeners[t];
          for (auto i : trigger)
          {
            rt->tryToCheckTrigger(i.first, i.second, t, rt->id);
          }
        }
        if ((rt->instCounter + 1) == t)
        {
          break;
        }
        // printf("popping from t.q of vm : %s, t : %d\n", rt->id, t);
        rt->triggerListeners.erase(t);
        rt->triggerQueue.pop();
        if (rt->triggerQueue.empty())
        {
          break;
        }
        t = rt->triggerQueue.top();
        rt->triggerFront = t;
      }
    }
    if (rt->triggerQueue.empty())
    {
      rt->newTiggersPendingToAdd = false;
    }
    // rt->tirggerLock.unlock();
  }
  else if (rt->newTiggersPendingToAdd && (rt->triggerFront < rt->instCounter))
  {
    while (rt->triggerFront < rt->instCounter)
    {
      rt->triggerQueue.pop();
      rt->triggerFront = rt->triggerQueue.top();
    }
    if (rt->triggerQueue.empty())
    {
      rt->newTiggersPendingToAdd = false;
    }
  }
}

void runTask(function<void(void *)> task, int index)
{
  task(vms[index]);
}

void registerRuntime(Runtime *rt)
{
  vmMap.insert({rt->id, {rt->index, rt}});
  vms[rt->index] = rt;
}

struct Lock
{
public:
  std::mutex mut;
};

struct Task
{
public:
  int id;
  unordered_map<int, pair<bool, Task *>> inputs;
  unordered_map<int, Task *> outputs;
  uint64_t start;
  uint64_t end;
  int vmIndex;
  bool started = false;
};

std::mutex mainLock;
function<void(Task *)> execTask;

std::mutex execLocks[COUNT];

void doCritical()
{
  int keyCounter = 1;
  map<int, std::tuple<vector<std::string>, uint64_t, uint64_t, int, std::string>> allTasks{};
  map<std::string, Task *> taskRefs{};
  unordered_map<std::string, pair<Task *, Lock *>> resLocks{};
  unordered_map<std::string, Task *> startPoints{};
  auto resCounter = 0;
  auto c = 0;

  for (int index = 0; index < COUNT; index++)
  {
    Runtime *vm = vms[index];
    for (auto t : vm->syncTasks)
    {
      vector<std::string> resNums = std::get<0>(t);
      uint64_t start = std::get<1>(t);
      uint64_t end = std::get<2>(t);
      std::string name = std::get<3>(t);
      std::stringstream mySS;
      int resCount = 0;
      for (auto r : resNums)
      {
        if (resLocks.find(r) == resLocks.end())
        {
          resLocks[r] = {NULL, new Lock()};
          resCounter++;
        }
        resCount++;
      }
      allTasks[keyCounter] = {resNums, start, end, index, name};
      keyCounter++;
      c++;
    }
  }

  for (auto i = 1; i <= keyCounter; i++)
  {
    vector<std::string> resNums = std::get<0>(allTasks[i]);
    unordered_map<int, pair<bool, Task *>> inputs = {};
    unordered_map<int, Task *> outputs = {};
    uint64_t start = std::get<1>(allTasks[i]);
    uint64_t end = std::get<2>(allTasks[i]);
    int vmIndex = std::get<3>(allTasks[i]);
    std::string name = std::get<4>(allTasks[i]);
    Task *task = new Task{i, inputs, outputs, start, end, vmIndex};
    std::string vmIndexStr = std::to_string(vmIndex);
    taskRefs[vmIndexStr + ":" + name] = task;
    for (auto r : resNums)
    {
      if (Utils::startswith(r, "lock_"))
      {
        Task *t = taskRefs[vmIndexStr + ":" + r];
        task->inputs[t->id] = {false, t};
        t->outputs[task->id] = task;
      }
      else
      {
        if (resLocks[r].first == NULL)
        {
          resLocks[r].first = task;
          startPoints[r] = task;
        }
        else
        {
          task->inputs[resLocks[r].first->id] = {false, resLocks[r].first};
          resLocks[r].first->outputs[task->id] = task;
          resLocks[r].first = task;
        }
      }
    }
  }

  std::mutex threadLocks[c];
  vector<thread> ts{};

  ThreadPool pool(resCounter);
  atomic<int> doneTasksCount = 1;

  execTask = [&pool, &doneTasksCount, keyCounter](Task *task)
  {
    bool readyToExec = false;
    mainLock.lock();
    if (task->started == false)
    {
      bool passed = true;
      for (auto t : task->inputs)
      {
        if (!t.first)
        {
          passed = false;
          break;
        }
      }
      if (passed)
      {
        task->started = true;
        readyToExec = true;
      }
    }
    mainLock.unlock();
    if (readyToExec)
    {
      pool.enqueue([task, &doneTasksCount, keyCounter]
                   {
                  printf("task %d\n", task->id);
                  execLocks[task->vmIndex].lock();
                  runTask([task](void *vmRaw)
                  {
                    void *res;
                    Runtime* vm = (Runtime*) vmRaw;
                    vm->prepare(task->start, task->end, res);
                    vm->execute(true);
                  },
                  task->vmIndex);
                  execLocks[task->vmIndex].unlock();
                  mainLock.lock();
                  doneTasksCount++;
                  if (doneTasksCount == keyCounter) {
                    endProgram();
                  }
                  vector<Task*> nextTasks{};
                  for (auto t : task->outputs) {
                    if (!t.second->started) {
                      t.second->inputs[task->id].first = true;
                      nextTasks.push_back(t.second);
                    }
                  }
                  mainLock.unlock();
                  for (auto t : nextTasks) {
                    execTask(t);
                  } });
    }
  };
  for (auto q : startPoints)
  {
    auto task = q.second;
    execTask(task);
  }
}