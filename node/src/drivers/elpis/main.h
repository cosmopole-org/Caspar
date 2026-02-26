
#ifdef __cplusplus
extern "C"
{
#endif
    char *elpisCallback(char *);
    void runVm(
        const char *astPath,
        const char *sendType,
        const char *pointId,
        const char *userId,
        const char *inputData
    );
#ifdef __cplusplus
} // extern "C"
#endif
