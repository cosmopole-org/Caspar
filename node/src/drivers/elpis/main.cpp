#include <stdio.h>
#include <unordered_map>
#include <string>
#include <list>
#include <unistd.h>
#include <mutex>
#include <string.h>
#include <any>
#include <unordered_set>
#include <condition_variable>
#include <queue>
#include <thread>
#include <functional>
#include <iostream>
#include <fstream>

#include "main.h"

#include "lib/runtime.cpp"

using namespace std;

void wait_all(list<std::shared_future<void>> futures)
{
  for (auto f : futures)
  {
    f.wait();
  }
}

char *vmIds[COUNT]{};
std::unordered_map<uint32_t, std::unordered_map<char *, bool>> resourceReqMap = {};
std::unordered_map<uint32_t, char *> lockMap = {};
std::unordered_map<uint32_t, uint64_t> resourceMap = {};
std::unordered_map<char *, uint32_t> shouldNotifyMap = {};

void createResource(uint32_t key, uint64_t val)
{
  globalLock.lock();
  resourceMap.insert({key, val});
  lockMap.insert({key, ""});
  resourceReqMap.insert({key, std::unordered_map<char *, bool>{}});
  globalLock.unlock();
}

void fillResource(int n)
{
  // printf("creating resources...\n");
  for (int i = 1; i <= n; i++)
  {
    createResource(i, i * 100);
  }
}

void promiseResource(uint32_t key, char *vmId)
{
  resourceReqMap[key][vmId] = true;
  // printf("%s waits for resource %d\n", vmId, key);
}

bool lockResource(uint32_t key, char *vmId)
{
  // printf("checking lock of %d %s\n", key, lockMap.find(key)->second);
  if (lockMap.find(key)->second == "")
  {
    lockMap[key] = vmId;
    resourceReqMap[key].erase(vmId);
    printf("%s captured resource %d\n", vmId, key);
    return true;
  }
  else
  {
    resourceReqMap[key][vmId] = true;
    // printf("%s waits for resource %d\n", vmId, key);
    return false;
  }
}

bool unlockResource(char *vmId, uint32_t key)
{
  if (lockMap.find(key)->second == vmId)
  {
    resourceReqMap[key].erase(vmId);
    lockMap[key] = "";
    printf("%s released resource %d\n", vmId, key);
    return true;
  }
  return false;
}

int checkForVmResume(char *vmId, uint32_t resourceNum, uint64_t instNum, char *myId)
{
  // printf("check any chance to resume vm %s and catch res %d\n", vmId, resourceNum);
  int vmIndex = vmMap.find(vmId)->second.first;
  auto isBeforeAll = true;
  Runtime *vm = vmMap.find(vmId)->second.second;
  globalLock.lock();
  auto progress = vm->instCounter;
  if (vm->paused && ((instNum == 0) || ((progress + 1) == instNum)))
  {
    unordered_map<char *, bool> equals{};
    for (auto n : vmMap)
    {
      if (n.first != vmId)
      {
        auto p = n.second.second->instCounter;
        if (p < progress)
        {
          isBeforeAll = false;
          break;
        }
        else if ((p == progress) && (n.first != myId))
        {
          equals[n.first] = true;
        }
      }
    }
    bool suitable = isBeforeAll;
    if (suitable)
    {
      if (equals.size() > 0)
      {
        for (auto b : equals)
        {
          if (b.first == vmId)
          {
            continue;
          }
          if (vmMap.find(b.first)->second.first < vmIndex)
          {
            suitable = false;
            break;
          }
        }
      }
    }
    if (suitable)
    {
      // printf("%s is behind\n", vmId);
      auto r = lockResource(resourceNum, vmId);
      if (r)
      {
        vm->resume(instNum);
        globalLock.unlock();
        return 0;
      }
      else
      {
        globalLock.unlock();
        return 1;
      }
    }
    globalLock.unlock();
    return 2;
  }
  else
  {
    if (progress <= instNum)
    {
      globalLock.unlock();
      return 0;
    }
    else
    {
      globalLock.unlock();
      return 3;
    }
  }
}

bool areAllPassed(uint64_t instCounter, std::unordered_map<char *, uint32_t> waiters, char *myId)
{
  bool allPassed = true;
  for (auto n : vmMap)
  {
    if (n.first == myId)
    {
      continue;
    }
    if (n.second.second->instCounter < instCounter)
    {
      if ((waiters.find(n.first) == waiters.end()) || (n.second.second->instCounter + 1 < instCounter))
      {
        // printf("%d %d", n.second.second->instCounter, instCounter);
        allPassed = false;
        break;
      }
    }
  }
  return allPassed;
}

bool areAllPassed2(uint64_t instCounter, std::unordered_map<char *, uint32_t> waiters, char *myId)
{
  return true;
}

void tryToCheckTrigger(char *vmId, uint32_t resNum, uint64_t instNum, char *myId)
{
  checkForVmResume(vmId, resNum, instNum, myId);
}

int onLockCalled(char *vmId, uint32_t resourceNum, bool shouldLock)
{
  if (shouldLock)
    globalLock.lock();
  auto vmIndex = vmMap.find(vmId)->second.first;
  // printf("lock called on %s\n", vmId);
  bool isBeforeAll = true;
  unordered_map<char *, bool> equals{};
  auto resource = resourceMap.find(resourceNum);
  if (resource != resourceMap.end())
  {
    bool shouldBlock = false;
    // printf("locking by %s\n", vmId);
    Runtime *vm = vmMap.find(vmId)->second.second;
    auto progress = vm->instCounter;
    for (auto n : vmMap)
    {
      if (n.first != vmId)
      {
        auto p = n.second.second->instCounter;
        // printf("%d %d \n", p, progress);
        if (p < progress)
        {
          isBeforeAll = false;
          break;
        }
        else if (p == progress)
        {
          equals[n.first] = true;
        }
      }
    }
    if (isBeforeAll)
    {
      bool suitable = true;
      if (equals.size() > 0)
      {
        for (auto b : equals)
        {
          if (b.first == vmId)
          {
            continue;
          }
          if (vmMap.find(b.first)->second.first < vmIndex)
          {
            suitable = false;
            break;
          }
        }
      }
      if (suitable)
      {
        // printf("%s is behind\n", vmId);
        auto r = lockResource(resourceNum, vmId);
        if (!r)
        {
          shouldBlock = true;
        }
      }
      else
      {
        // printf("%s promising resource...\n", vmId);
        promiseResource(resourceNum, vmId);
        for (auto n : vmMap)
        {
          if (n.first == vmId)
          {
            continue;
          }
          n.second.second->addTrigger(progress + 1, vmId, resourceNum);
        }
        shouldBlock = true;
      }
    }
    else
    {
      resourceReqMap[resourceNum][vmId] = true;
      for (auto n : vmMap)
      {
        if (n.first == vmId)
        {
          continue;
        }
        n.second.second->addTrigger(progress + 1, vmId, resourceNum);
      }
      // printf("%s waits for resource %d\n", vmId, resourceNum);
      shouldBlock = true;
    }
    if (shouldBlock)
    {
      vm->pause();
      if (shouldLock)
        globalLock.unlock();
      return 1;
    }
  }
  if (shouldLock)
    globalLock.unlock();
  return 0;
}

void onUnlockCalled(char *unLockerVmId, uint32_t resourceNum)
{
  globalLock.lock();
  auto resource = resourceMap.find(resourceNum);
  if (resource != resourceMap.end())
  {
    // printf("un-lock called on %s\n", unLockerVmId);
    if (unlockResource(unLockerVmId, resourceNum) && !resourceReqMap[resourceNum].empty())
    {
      for (auto m : resourceReqMap[resourceNum])
      {
        auto vmIndex = vmMap.find(m.first)->second.first;
        bool isBeforeAll = true;
        unordered_map<char *, bool> equals{};
        auto resource = resourceMap.find(resourceNum);
        bool shouldBlock = false;
        Runtime *vm = vmMap.find(m.first)->second.second;
        auto progress = vm->instCounter;
        for (auto n : vmMap)
        {
          if (n.first != m.first)
          {
            auto p = n.second.second->instCounter;
            // printf("%d %d \n", p, progress);
            if (p < progress)
            {
              isBeforeAll = false;
              break;
            }
            else if (p == progress)
            {
              equals[n.first] = true;
            }
          }
        }
        if (isBeforeAll)
        {
          bool suitable = true;
          if (equals.size() > 0)
          {
            for (auto b : equals)
            {
              if (b.first == m.first)
              {
                continue;
              }
              if (vmMap.find(b.first)->second.first < vmIndex)
              {
                suitable = false;
                break;
              }
            }
          }
          if (suitable)
          {
            // printf("%s is behind\n", m.first);
            auto r = lockResource(resourceNum, m.first);
            if (!r)
            {
              shouldBlock = true;
            }
          }
          else
          {
            // printf("%s promising resource...\n", m.first);
            promiseResource(resourceNum, m.first);
            shouldNotifyMap[m.first] = resourceNum;
            shouldBlock = true;
          }
        }
        else
        {
          resourceReqMap[resourceNum][m.first] = true;
          for (auto n : vmMap)
          {
            if (n.first == m.first)
            {
              continue;
            }
            n.second.second->addTrigger(progress + 1, m.first, resourceNum);
          }
          // printf("%s waits for resource %d\n", m.first, resourceNum);
          shouldBlock = true;
        }
        if (shouldBlock)
        {
          continue;
        }
        else
        {
          vmMap.find(m.first)->second.second->resume(0);
          break;
        }
      }
    }
  }
  globalLock.unlock();
}

int onLockCalled2(char *vmId, uint32_t resourceNum, bool shouldLock)
{
  return 0;
}

void onUnlockCalled2(char *vmId, uint32_t resourceNum) {}

const uint64_t CODE_SIZE = 160000;

void tryToCheckTrigger2(char *vmId, uint32_t resNum, uint64_t instNum, char *myId) {}

void testSequential()
{
  for (int index = 0; index < COUNT; index++)
  {
    auto rt = vmMap.find(vmIds[index])->second.second;
    int *res;
    rt->prepare(0, CODE_SIZE, res);
    rt->execute(true);
  }
}

void createVms()
{
  vmMap = std::unordered_map<char *, std::pair<int, Runtime *>>{};

  for (int index = 0; index < COUNT; index++)
  {
    auto temp_str = std::to_string(index);
    char *char_type = new char[temp_str.length()];
    strcpy(char_type, temp_str.c_str());
    vmIds[index] = char_type;
    char *id = vmIds[index];
    Operation *code[160000];
    for (int counter = 0; counter < 20000; counter++)
    {
      code[counter] = new DefineVar("a", i32_t, 1);
    }
    code[20000] = new DefineVar("b", i32_t, 1);
    code[20001] = new LockData("lock_a", {std::to_string(index + 1)});
    code[20002] = new VarPlusPlus({iden_t, Id("a")});
    code[20003] = new UnlockData("lock_a", {std::to_string(index + 1)});
    for (int counter = 20004; counter < 100000; counter++)
    {
      code[counter] = new DefineVar("a", i32_t, 1);
    }
    code[100000] = new LockData("lock_b", {std::to_string(index + 1), "lock_a"});
    code[100001] = new VarPlusPlus({iden_t, Id("a")});
    code[100002] = new UnlockData("lock_b", {std::to_string(index + 1), "lock_a"});
    for (int counter = 100003; counter < 160000; counter++)
    {
      code[counter] = new DefineVar("a", i32_t, 1);
    }
    registerRuntime(new Runtime(
        id,
        index,
        code,
        CODE_SIZE,
        areAllPassed,
        tryToCheckTrigger,
        onLockCalled,
        onUnlockCalled));
  }
}

void createVms2()
{
  vmMap = std::unordered_map<char *, std::pair<int, Runtime *>>{};

  for (int index = 0; index < COUNT; index++)
  {
    auto temp_str = std::to_string(index);
    char *char_type = new char[temp_str.length()];
    strcpy(char_type, temp_str.c_str());
    vmIds[index] = char_type;
    char *id = vmIds[index];
    Operation *code[160000];
    for (int counter = 0; counter < 20000; counter++)
    {
      code[counter] = new DefineVar("a", i32_t, 1);
    }
    code[20000] = new DefineVar("b", i32_t, 1);
    code[20001] = new LockData("lock_a", {std::to_string(index + 1)});
    code[20002] = new VarPlusPlus({iden_t, Id("a")});
    code[20003] = new UnlockData("lock_a", {std::to_string(index + 1)});
    for (int counter = 20004; counter < 100000; counter++)
    {
      code[counter] = new DefineVar("a", i32_t, 1);
    }
    code[100000] = new LockData("lock_b", {std::to_string(index + 1), "lock_a"});
    code[100001] = new VarPlusPlus({iden_t, Id("a")});
    code[100002] = new UnlockData("lock_b", {std::to_string(index + 1), "lock_a"});
    for (int counter = 100003; counter < 160000; counter++)
    {
      code[counter] = new DefineVar("a", i32_t, 1);
    }
    registerRuntime(new Runtime(
        id,
        index,
        code,
        CODE_SIZE,
        areAllPassed2,
        tryToCheckTrigger2,
        onLockCalled2,
        onUnlockCalled2));
  }
}

std::chrono::_V2::system_clock::time_point start1;
std::chrono::_V2::system_clock::time_point start2;
std::chrono::_V2::system_clock::time_point stop1;
std::chrono::_V2::system_clock::time_point stop2;
int64_t time1;
int64_t time2;

bool allowEnd = false;

void endProgram()
{
  if (allowEnd)
  {
    stop1 = std::chrono::high_resolution_clock::now();
    time1 = std::chrono::duration_cast<std::chrono::microseconds>(stop1 - start1).count();

    printf("----------------------------------------------------------------\n");

    printf("parallel:    %d\n", time1);
    printf("sequential:  %d\n", time2);

    exit(0);
  }
}

// int main(int Argc, const char *Argv[])
// {
//   fillResource(COUNT);

//   createVms2();
//   start2 = std::chrono::high_resolution_clock::now();
//   testSequential();
//   stop2 = std::chrono::high_resolution_clock::now();
//   time2 = std::chrono::duration_cast<std::chrono::microseconds>(stop2 - start2).count();

//   printf("----------------------------------------------------------------\n");

//   allowEnd = true;
//   doneTasks = 0;

//   createVms();

//   start1 = std::chrono::high_resolution_clock::now();
//   for (auto m : vmMap)
//   {
//     // printf("starting vm %s...\n", m.first);
//     int *res;
//     m.second.second->run(0, CODE_SIZE, res);
//   }

//   thread([]
//          {
//     sleep(3);
//     exit(1); })
//       .join();

//   for (auto m : vmMap)
//   {
//     m.second.second->stick();
//   }

//   return 0;
// }

pair<uint32_t, any>* parseJsonObject(json jsonObj)
{
  if (jsonObj.is_number_integer())
  {
    auto val = jsonObj.template get<long>();
    return new pair<uint32_t, any>(i64_t, val);
  }
  else if (jsonObj.is_number_float())
  {
    auto val = jsonObj.template get<double>();
    return new pair<uint32_t, any>(i32_t, val);
  }
  else if (jsonObj.is_string())
  {
    auto val = jsonObj.template get<std::string>();
    return new pair<uint32_t, any>(str_t, val);
  }
  else if (jsonObj.is_boolean())
  {
    auto val = jsonObj.template get<bool>();
    return new pair<uint32_t, any>(bool_t, val);
  }
  else if (jsonObj.is_array())
  {
    vector<pair<uint32_t, any>*> arr{};
    for (json::iterator item = jsonObj.begin(); item != jsonObj.end(); ++item)
    {
      arr.push_back(parseJsonObject(item.value()));
    }
    return new pair<uint32_t, any>(arr_t, Ref(2, new Array(arr)));
  }
  else if (jsonObj.is_object())
  {
    map<string, pair<uint32_t, any>*> res{};
    for (json::iterator item = jsonObj.begin(); item != jsonObj.end(); ++item)
    {
      res[item.key()] = parseJsonObject(item.value());
    }
    return new pair<uint32_t, any>(obj_t, Ref(1, new Object(res)));
  }
  return new pair<uint32_t, any>(und_t, NULL);
}

void runVm(
  const char *astPath,
  const char *sendType,
  const char *pointId,
  const char *userId,
  const char *inputData)
{
  FILE *file = fopen(astPath, "r+");
  if (file == NULL)
  {
    printf("file not found\n");
    return;
  }
  fseek(file, 0, SEEK_END);
  long int size = ftell(file);
  fclose(file);
  file = fopen(astPath, "r+");
  unsigned char *in = (unsigned char *)malloc(size);
  int bytes_read = fread(in, sizeof(unsigned char), size, file);
  fclose(file);
  auto rt = new Runtime(in, bytes_read, elpisCallback);
  int *res;
  rt->prepare(1 + 1, bytes_read + 1 + 1 + 1, res);
  json inp = json::parse(inputData);
  rt->executeOnUpdate(sendType, pointId, userId, any_cast<Ref>(parseJsonObject(inp)->second), true);
}
